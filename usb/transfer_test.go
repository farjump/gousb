// Copyright 2017 the gousb Authors.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package usb

import (
	"testing"
	"time"
)

func TestNewTransfer(t *testing.T) {
	defer func(i libusbIntf) { libusb = i }(libusb)
	libusb = newFakeLibusb()

	for _, tc := range []struct {
		desc        string
		dir         EndpointDirection
		tt          TransferType
		maxPkt      uint32
		buf         int
		timeout     time.Duration
		wantIso     int
		wantLength  int
		wantTimeout int
	}{
		{
			desc:       "bulk in transfer, 512B packets",
			dir:        EndpointDirectionIn,
			tt:         TransferTypeBulk,
			maxPkt:     512,
			buf:        1024,
			timeout:    time.Second,
			wantLength: 1024,
		},
		{
			desc:       "iso out transfer, 3 * 1024B packets",
			dir:        EndpointDirectionOut,
			tt:         TransferTypeIsochronous,
			maxPkt:     3 * 1024,
			buf:        10000,
			wantLength: 10000,
		},
	} {
		xfer, err := newUSBTransfer(nil, &EndpointInfo{
			Number:        2,
			Direction:     tc.dir,
			TransferType:  tc.tt,
			MaxPacketSize: tc.maxPkt,
		}, make([]byte, tc.buf), tc.timeout)

		if err != nil {
			t.Fatalf("newUSBTransfer(): %v", err)
		}
		if got, want := len(xfer.buf), tc.wantLength; got != want {
			t.Errorf("xfer.buf: got %d bytes, want %d", got, want)
		}
	}
}

func TestTransferProtocol(t *testing.T) {
	defer func(i libusbIntf) { libusb = i }(libusb)

	f := newFakeLibusb()
	libusb = f

	xfers := make([]*usbTransfer, 2)
	var err error
	for i := 0; i < 2; i++ {
		xfers[i], err = newUSBTransfer(nil, &EndpointInfo{
			Number:        6,
			Direction:     EndpointDirectionIn,
			TransferType:  TransferTypeBulk,
			MaxPacketSize: 512,
		}, make([]byte, 10240), time.Second)
		if err != nil {
			t.Fatalf("newUSBTransfer: %v", err)
		}
	}

	go func() {
		ft := f.waitForSubmitted()
		ft.length = 5
		ft.status = LIBUSB_TRANSFER_COMPLETED
		copy(ft.buf, []byte{1, 2, 3, 4, 5})
		close(ft.done)

		ft = f.waitForSubmitted()
		ft.length = 99
		ft.status = LIBUSB_TRANSFER_COMPLETED
		copy(ft.buf, []byte{12, 12, 12, 12, 12})
		close(ft.done)

		ft = f.waitForSubmitted()
		ft.length = 123
		ft.status = LIBUSB_TRANSFER_CANCELLED
		close(ft.done)
	}()

	xfers[0].submit()
	xfers[1].submit()
	got, err := xfers[0].wait()
	if err != nil {
		t.Errorf("xfer#0.wait returned error %v, want nil", err)
	}
	if want := 5; got != want {
		t.Errorf("xfer#0.wait returned %d bytes, want %d", got, want)
	}
	got, err = xfers[1].wait()
	if err != nil {
		t.Errorf("xfer#0.wait returned error %v, want nil", err)
	}
	if want := 99; got != want {
		t.Errorf("xfer#0.wait returned %d bytes, want %d", got, want)
	}

	xfers[1].submit()
	xfers[1].cancel()
	got, err = xfers[1].wait()
	if err == nil {
		t.Error("xfer#1(resubmitted).wait returned error nil, want non-nil")
	}
	if want := 123; got != want {
		t.Errorf("xfer#1(resubmitted).wait returned %d bytes, want %d", got, want)
	}

	for _, x := range xfers {
		x.cancel()
		x.wait()
		x.free()
	}
}
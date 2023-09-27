/*
Copyright 2022 The Alibaba Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package shm

import (
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestNewOpen(t *testing.T) {
	for d := uint(1); d <= 30; d++ {
		size := int32(1) << d

		// create shared memory
		w, err := Create("test_t", size)
		if err != nil {
			t.Errorf("warn: fail create %d byte shared memroy %v", size, err)
			continue
		}

		// open shared memory
		r, err := Open("test_t", size)
		if err != nil {
			w.Close()
			t.Errorf("warn: fail open %d byte shared memroy %v", size, err)
			continue
		}

		w.Close()
		r.Close()
	}
}

func TestReadWriteAt(t *testing.T) {
	tests := []struct {
		size int
		data string
	}{
		{size: 1, data: "a"},                      // single
		{size: 63, data: strings.Repeat("a", 63)}, // full - 1
		{size: 64, data: strings.Repeat("b", 64)}, // full
		{size: 64, data: strings.Repeat("c", 65)}, // shrink
	}

	// create shared memory
	w, _ := Create("test_t", 64)
	defer w.Close()

	// open shared memory
	r, _ := Open("test_t", 64)
	defer r.Close()

	// read/write test
	for _, tt := range tests {
		data := []byte(tt.data)

		n, err := w.WriteAt(data, 0)
		if err != nil {
			t.Fatalf("fail: write shared memroy %v", err)
		}
		if n != tt.size {
			t.Fatalf("fail: write shared memroy %d byte, want %d byte", n, tt.size)
		}

		buf := make([]byte, len(data))
		n, err = r.ReadAt(buf, 0)
		if err != nil {
			t.Fatalf("fail: read shared memroy %v", err)
		}
		if n != tt.size {
			t.Fatalf("fail: read shared memroy %d byte, want %d byte", n, tt.size)
		}

		t.Logf("info: read shared memroy %v", buf[:tt.size])
		t.Logf("info: expect shared memroy %v", data[:tt.size])
		if !reflect.DeepEqual(buf[:tt.size], data[:tt.size]) {
			t.Fatalf("fail: read shared memroy %v, want %v", buf[:tt.size], data[:tt.size])
		}
	}
}

func TestReadWriteAt_OverPosition(t *testing.T) {
	tests := []struct {
		pos  int
		succ bool
	}{
		{pos: 0, succ: true},
		{pos: 63, succ: true},
		{pos: 64, succ: false},
	}

	// create shared memory
	w, _ := Create("test_t", 64)
	defer w.Close()

	// open shared memory
	r, _ := Open("test_t", 64)
	defer r.Close()

	// write dummy
	{
		data := []byte(strings.Repeat("a", 64))
		n, err := w.WriteAt(data, 0)
		if err != nil {
			t.Fatalf("fail: write shared memroy %v", err)
		}
		if n != 64 {
			t.Fatalf("fail: write shared memroy %d byte, want %d byte", n, 64)
		}
	}

	// read/write test
	for _, tt := range tests {
		data := []byte("b")
		n, err := w.WriteAt(data, int64(tt.pos))
		if tt.succ {
			// success
			if err != nil {
				t.Fatalf("fail: write shared memroy %v", err)
			}
			if n != 1 {
				t.Fatalf("fail: write shared memroy %d byte, want %d byte", n, 1)
			}
		} else {
			// fail
			if err != io.EOF {
				t.Fatalf("fail: write shared memroy raise %v, want %v", err, io.EOF)
			}
		}

		buf := make([]byte, 1)
		n, err = r.ReadAt(buf, int64(tt.pos))
		if tt.succ {
			// success
			if err != nil {
				t.Fatalf("fail: read shared memroy %v", err)
			}
			if n != 1 {
				t.Fatalf("fail: read shared memroy %d byte, want %d byte", n, 1)
			}

			t.Logf("info: read shared memroy %v", buf)
			t.Logf("info: expect shared memroy %v", data)
			if !reflect.DeepEqual(buf, data) {
				t.Fatalf("fail: read shared memroy %v, want %v", buf, data)
			}
		} else {
			// fail
			if err != io.EOF {
				t.Fatalf("fail: read shared memroy raise %v, want %v", err, io.EOF)
			}
		}
	}
}

func TestReadWriteAt_MultiThreads(t *testing.T) {
	tests := []struct {
		size int
		data string
	}{
		{size: 1, data: "a"},                      // single
		{size: 63, data: strings.Repeat("a", 63)}, // full - 1
		{size: 64, data: strings.Repeat("b", 64)}, // full
		{size: 64, data: strings.Repeat("c", 65)}, // shrink
	}

	// create shared memory
	w, _ := Create("test_t", 64)
	defer w.Close()

	// open shared memory
	r, _ := Open("test_t", 64)
	defer r.Close()

	wg := new(sync.WaitGroup)
	written := make(chan bool)
	readone := make(chan bool)

	// write thread
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, tt := range tests {
			// write data
			data := []byte(tt.data)
			n, err := w.WriteAt(data, 0)
			if err != nil {
				written <- false
				t.Fatalf("fail: write shared memroy %v", err)
			}
			if n != tt.size {
				written <- false
				t.Fatalf("fail: write shared memroy %d byte, want %d byte", n, tt.size)
			}
			written <- true

			// wait
			succ := <-readone
			if !succ {
				return
			}
		}
	}()

	// read thread
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, tt := range tests {
			// wait
			succ := <-written
			if !succ {
				return
			}

			// read data
			data := []byte(tt.data)
			buf := make([]byte, len(data))
			n, err := r.ReadAt(buf, 0)
			if err != nil {
				readone <- false
				t.Fatalf("fail: read shared memroy %v", err)
			}
			if n != tt.size {
				readone <- false
				t.Fatalf("fail: read shared memroy %d byte, want %d byte", n, tt.size)
			}

			t.Logf("info: read shared memroy %v", buf[:tt.size])
			t.Logf("info: expect shared memroy %v", data[:tt.size])
			if !reflect.DeepEqual(buf[:tt.size], data[:tt.size]) {
				readone <- false
				t.Fatalf("fail: read shared memroy %v, want %v", buf[:tt.size], data[:tt.size])
			}
			readone <- true
		}
	}()

	wg.Wait()
}

func TestReadWrite(t *testing.T) {
	tests := []struct {
		succ bool
		data string
	}{
		{succ: true, data: "a"},                     // single
		{succ: true, data: strings.Repeat("b", 63)}, // full
		{succ: false, data: "c"},                    // overflow (EOF)
	}

	// create shared memory
	w, _ := Create("test_t", 64)
	defer w.Close()

	// open shared memory
	r, _ := Open("test_t", 64)
	defer r.Close()

	// read/write test
	for _, tt := range tests {
		data := []byte(tt.data)

		n, err := w.Write(data)
		if tt.succ {
			// success
			if err != nil {
				t.Fatalf("fail: write shared memroy %v", err)
			}
			if n != len(data) {
				t.Fatalf("fail: write shared memroy %d byte, want %d byte", n, len(data))
			}
		} else {
			// fail
			if err != io.EOF {
				t.Fatalf("fail: write shared memroy raise %v, want %v", err, io.EOF)
			}
		}

		buf := make([]byte, len(data))
		n, err = r.Read(buf)
		if tt.succ {
			// success
			if err != nil {
				t.Fatalf("fail: read shared memroy %v", err)
			}
			if n != len(data) {
				t.Fatalf("fail: read shared memroy %d byte, want %d byte", n, len(data))
			}

			t.Logf("info: read shared memroy %v", buf)
			t.Logf("info: expect shared memroy %v", data)
			if !reflect.DeepEqual(buf, data) {
				t.Fatalf("fail: read shared memroy %v, want %v", buf, data)
			}
		} else {
			// fail
			if err != io.EOF {
				t.Fatalf("fail: read shared memroy raise %v, want %v", err, io.EOF)
			}
		}
	}
}

func TestReadWrite_MultiThreads(t *testing.T) {
	tests := []struct {
		size int
		data string
	}{
		{size: 1, data: "a"},                      // single
		{size: 62, data: strings.Repeat("a", 62)}, // full - 1
		{size: 1, data: strings.Repeat("b", 10)},  // shrink
	}

	// create shared memory
	w, _ := Create("test_t", 64)
	defer w.Close()

	// open shared memory
	r, _ := Open("test_t", 64)
	defer r.Close()

	wg := new(sync.WaitGroup)
	written := make(chan bool)
	readone := make(chan bool)

	// write thread
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, tt := range tests {
			// write data
			data := []byte(tt.data)
			n, err := w.Write(data)
			if err != nil {
				written <- false
				t.Fatalf("fail: write shared memroy %v", err)
			}
			if n != tt.size {
				written <- false
				t.Fatalf("fail: write shared memroy %d byte, want %d byte", n, tt.size)
			}
			written <- true

			// wait
			succ := <-readone
			if !succ {
				return
			}
		}
	}()

	// read thread
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, tt := range tests {
			// wait
			succ := <-written
			if !succ {
				return
			}

			// read data
			data := []byte(tt.data)
			buf := make([]byte, len(data))
			n, err := r.Read(buf)
			if err != nil {
				readone <- false
				t.Fatalf("fail: read shared memroy %v", err)
			}
			if n != tt.size {
				readone <- false
				t.Fatalf("fail: read shared memroy %d byte, want %d byte", n, tt.size)
			}

			t.Logf("info: read shared memroy %v", buf[:tt.size])
			t.Logf("info: expect shared memroy %v", data[:tt.size])
			if !reflect.DeepEqual(buf[:tt.size], data[:tt.size]) {
				readone <- false
				t.Fatalf("fail: read shared memroy %v, want %v", buf[:tt.size], data[:tt.size])
			}
			readone <- true
		}
	}()

	wg.Wait()
}

// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package paging

import (
	"sync"
	"testing"
)

func TestLiveEngineSwap(t *testing.T) {
	a, b := &Engine{}, &Engine{}
	l := NewLiveEngine(a)
	if l.Current() != a {
		t.Fatal("want the first engine")
	}
	l.Swap(b)
	if l.Current() != b {
		t.Fatal("want the swapped engine")
	}
}

func TestLiveEngineConcurrentSwapAndRead(t *testing.T) {
	engines := []*Engine{{}, {}, {}}
	l := NewLiveEngine(engines[0])
	var wg sync.WaitGroup
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20000; i++ {
				if l.Current() == nil {
					t.Error("nil engine")
					return
				}
			}
		}()
	}
	for i := 0; i < 2000; i++ {
		l.Swap(engines[i%len(engines)])
	}
	wg.Wait()
}

// The hot path is one atomic load: no I/O and no allocation.
func TestLiveEngineCurrentAllocatesNothing(t *testing.T) {
	l := NewLiveEngine(&Engine{})
	if n := testing.AllocsPerRun(1000, func() { _ = l.Current() }); n != 0 {
		t.Fatalf("Current allocated %v times per call, want 0", n)
	}
}

func BenchmarkLiveEngineCurrent(b *testing.B) {
	l := NewLiveEngine(&Engine{})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = l.Current()
	}
}

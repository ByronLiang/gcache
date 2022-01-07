package gcache

import (
	"fmt"
	"testing"
	"time"
)

func TestLRUGet(t *testing.T) {
	size := 1000
	gc := buildTestCache(t, TYPE_LRU, size)
	testSetCache(t, gc, size)
	testGetCache(t, gc, size)
}

func TestLoadingLRUGet(t *testing.T) {
	size := 1000
	gc := buildTestLoadingCache(t, TYPE_LRU, size, loader)
	testGetCache(t, gc, size)
}

func TestLRULength(t *testing.T) {
	gc := buildTestLoadingCache(t, TYPE_LRU, 1000, loader)
	gc.Get("test1")
	gc.Get("test2")
	length := gc.Len(true)
	expectedLength := 2
	if length != expectedLength {
		t.Errorf("Expected length is %v, not %v", length, expectedLength)
	}
}

func TestLRUEvictItem(t *testing.T) {
	cacheSize := 10
	numbers := 11
	gc := buildTestLoadingCache(t, TYPE_LRU, cacheSize, loader)

	for i := 0; i < numbers; i++ {
		_, err := gc.Get(fmt.Sprintf("Key-%d", i))
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}
}

func TestLRUCache_GetKeyTTL(t *testing.T) {
	size := 10
	gc := buildTestCache(t, TYPE_LRU, size)
	err := gc.SetWithExpire("test-ttl", "test", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	ttl, err := gc.GetKeyTTL("test-ttl")
	if err != nil {
		t.Fatal(err)
	}
	if ttl != nil {
		t.Log((*ttl).Seconds())
	}
	err = gc.Set("none-ttl", "none")
	if err != nil {
		t.Fatal(err)
	}
	ttlNil, err := gc.GetKeyTTL("none-ttl")
	if err != nil {
		t.Fatal(err)
	}
	if ttlNil != nil {
		t.Log(ttlNil.Seconds())
	}
}

func TestLRUCache_BatchGet(t *testing.T) {
	size := 10
	gc := buildTestCache(t, TYPE_LRU, size)
	testSetCache(t, gc, size)
	testBatchGetCache(t, gc, []int{2, 3, 6, 4, 100})
}

func TestLRUGetIFPresent(t *testing.T) {
	testGetIFPresent(t, TYPE_LRU)
}

func TestLRUHas(t *testing.T) {
	gc := buildTestLoadingCacheWithExpiration(t, TYPE_LRU, 2, 10*time.Millisecond)

	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			gc.Get("test1")
			gc.Get("test2")

			if gc.Has("test0") {
				t.Fatal("should not have test0")
			}
			if !gc.Has("test1") {
				t.Fatal("should have test1")
			}
			if !gc.Has("test2") {
				t.Fatal("should have test2")
			}

			time.Sleep(20 * time.Millisecond)

			if gc.Has("test0") {
				t.Fatal("should not have test0")
			}
			if gc.Has("test1") {
				t.Fatal("should not have test1")
			}
			if gc.Has("test2") {
				t.Fatal("should not have test2")
			}
		})
	}
}

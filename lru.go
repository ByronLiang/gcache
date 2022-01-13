package gcache

import (
	"container/list"
	"time"
)

// Discards the least recently used items first.
type LRUCache struct {
	baseCache
	items     map[interface{}]*lruItem
	evictList *list.List
}

func newLRUCache(cb *CacheBuilder) *LRUCache {
	c := &LRUCache{}
	buildCache(&c.baseCache, cb)

	c.init()
	c.loadGroup.cache = c
	return c
}

func (c *LRUCache) init() {
	c.evictList = list.New()
	c.items = make(map[interface{}]*lruItem, c.size+1)
}

func (c *LRUCache) set(key, value interface{}) (interface{}, error) {
	var err error
	if c.serializeFunc != nil {
		value, err = c.serializeFunc(key, value)
		if err != nil {
			return nil, err
		}
	}

	// Check for existing item
	item, ok := c.items[key]
	if ok {
		c.evictList.MoveToFront(item.element)
		item.value = value
	} else {
		// Verify size not exceeded
		if c.evictList.Len() >= c.size {
			c.evict(1)
		}
		item = &lruItem{
			clock:   c.clock,
			key:     key,
			value:   value,
			element: nil,
		}
		ele := c.evictList.PushFront(item)
		item.element = ele
		c.items[key] = item
	}

	if c.expiration != nil {
		t := c.clock.Now().Add(*c.expiration)
		item.expiration = &t
	}

	if c.addedFunc != nil {
		c.addedFunc(key, value)
	}

	return item, nil
}

// set a new key-value pair
func (c *LRUCache) Set(key, value interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.set(key, value)
	return err
}

// Set a new key-value pair with an expiration time
func (c *LRUCache) SetWithExpire(key, value interface{}, expiration time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, err := c.set(key, value)
	if err != nil {
		return err
	}

	t := c.clock.Now().Add(expiration)
	item.(*lruItem).expiration = &t
	return nil
}

func (c *LRUCache) BatchSet(reqs []BatchSetReq) error {
	if len(reqs) > c.size {
		return KeyBatchSetOverCacheSize
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, batchSetReq := range reqs {
		item, err := c.set(batchSetReq.GetKey(), batchSetReq.GetValue())
		if err != nil {
			return err
		}
		if batchSetReq.GetExpiration() != nil {
			t := c.clock.Now().Add(*batchSetReq.GetExpiration())
			item.(*lruItem).expiration = &t
		}
	}
	return nil
}

// Get a value from cache pool using key if it exists.
// If it does not exists key and has LoaderFunc,
// generate a value using `LoaderFunc` method returns value.
func (c *LRUCache) Get(key interface{}) (interface{}, error) {
	v, err := c.get(key, false)
	if err == KeyNotFoundError {
		return c.getWithLoader(key, true)
	}
	return v, err
}

func (c *LRUCache) GetKeyTTL(key interface{}) (*time.Duration, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.items[key]
	if ok {
		duration, isSetExpire := item.GetExpirationDuration(nil)
		if isSetExpire {
			return duration, nil
		}
		return nil, KeyNotSetWithExpireError
	}
	return nil, KeyNotFoundError
}

// GetIFPresent gets a value from cache pool using key if it exists.
// If it does not exists key, returns KeyNotFoundError.
// And send a request which refresh value for specified key if cache object has LoaderFunc.
func (c *LRUCache) GetIFPresent(key interface{}) (interface{}, error) {
	v, err := c.get(key, false)
	if err == KeyNotFoundError {
		return c.getWithLoader(key, false)
	}
	return v, err
}

func (c *LRUCache) get(key interface{}, onLoad bool) (interface{}, error) {
	v, err := c.getValue(key, onLoad)
	if err != nil {
		return nil, err
	}
	if c.deserializeFunc != nil {
		return c.deserializeFunc(key, v)
	}
	return v, nil
}

func (c *LRUCache) getValue(key interface{}, onLoad bool) (interface{}, error) {
	c.mu.Lock()
	item, ok := c.items[key]
	if ok {
		if !item.IsExpired(nil) {
			c.evictList.MoveToFront(item.element)
			v := item.value
			c.mu.Unlock()
			if !onLoad {
				c.statsAccessor.IncrHitCount()
			}
			return v, nil
		}
		c.removeElement(item)
	}
	c.mu.Unlock()
	if !onLoad {
		c.statsAccessor.IncrMissCount()
	}
	return nil, KeyNotFoundError
}

func (c *LRUCache) getWithLoader(key interface{}, isWait bool) (interface{}, error) {
	if c.loaderExpireFunc == nil {
		return nil, KeyNotFoundError
	}
	value, _, err := c.load(key, func(v interface{}, expiration *time.Duration, e error) (interface{}, error) {
		if e != nil {
			return nil, e
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		item, err := c.set(key, v)
		if err != nil {
			return nil, err
		}
		if expiration != nil {
			t := c.clock.Now().Add(*expiration)
			item.(*lruItem).expiration = &t
		}
		return v, nil
	}, isWait)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// evict removes the oldest item from the cache.
func (c *LRUCache) evict(count int) {
	for i := 0; i < count; i++ {
		ent := c.evictList.Back()
		if ent == nil {
			return
		} else {
			entry := ent.Value.(*lruItem)
			c.removeElement(entry)
		}
	}
}

// Has checks if key exists in cache
func (c *LRUCache) Has(key interface{}) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	return c.has(key, &now)
}

func (c *LRUCache) has(key interface{}, now *time.Time) bool {
	item, ok := c.items[key]
	if !ok {
		return false
	}
	return !item.IsExpired(now)
}

// Remove removes the provided key from the cache.
func (c *LRUCache) Remove(key interface{}) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.remove(key)
}

func (c *LRUCache) remove(key interface{}) bool {
	if ent, ok := c.items[key]; ok {
		c.removeElement(ent)
		return true
	}
	return false
}

func (c *LRUCache) removeElement(entry *lruItem) {
	c.evictList.Remove(entry.element)
	delete(c.items, entry.key)
	if c.evictedFunc != nil {
		c.evictedFunc(entry.key, entry.value)
	}
}

func (c *LRUCache) keys() []interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]interface{}, len(c.items))
	var i = 0
	for k := range c.items {
		keys[i] = k
		i++
	}
	return keys
}

// GetALL returns all key-value pairs in the cache.
func (c *LRUCache) GetALL(checkExpired bool) map[interface{}]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	items := make(map[interface{}]interface{}, len(c.items))
	now := time.Now()
	for k, item := range c.items {
		if !checkExpired || !item.IsExpired(&now) {
			items[k] = item.value
		}
	}
	return items
}

func (c *LRUCache) BatchGet(checkExpired bool, keys []interface{}) map[interface{}]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	items := make(map[interface{}]interface{}, len(keys))
	now := time.Now()
	for _, k := range keys {
		if item, ok := c.items[k]; ok {
			if !checkExpired || !item.IsExpired(&now) {
				items[k] = item.value
			}
		} else {
			value, keyEmptyErr := c.getWithLoader(k, true)
			if keyEmptyErr == nil {
				items[k] = value
			}
		}
	}
	return items
}

// Keys returns a slice of the keys in the cache.
func (c *LRUCache) Keys(checkExpired bool) []interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]interface{}, 0, len(c.items))
	now := time.Now()
	for k := range c.items {
		if !checkExpired || c.has(k, &now) {
			keys = append(keys, k)
		}
	}
	return keys
}

// Len returns the number of items in the cache.
func (c *LRUCache) Len(checkExpired bool) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !checkExpired {
		return len(c.items)
	}
	var length int
	now := time.Now()
	for k := range c.items {
		if c.has(k, &now) {
			length++
		}
	}
	return length
}

// Completely clear the cache
func (c *LRUCache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.purgeVisitorFunc != nil {
		for key, item := range c.items {
			c.purgeVisitorFunc(key, item.value)
		}
	}

	c.init()
}

type lruItem struct {
	clock      Clock
	key        interface{}
	value      interface{}
	expiration *time.Time
	element    *list.Element
}

// IsExpired returns boolean value whether this item is expired or not.
func (it *lruItem) IsExpired(now *time.Time) bool {
	if it.expiration == nil {
		return false
	}
	if now == nil {
		t := it.clock.Now()
		now = &t
	}
	return it.expiration.Before(*now)
}

func (it *lruItem) GetExpirationDuration(now *time.Time) (*time.Duration, bool) {
	if it.expiration == nil {
		return nil, false
	}
	if now == nil {
		t := it.clock.Now()
		now = &t
	}
	duration := (*it.expiration).Sub(*now)
	return &duration, true
}

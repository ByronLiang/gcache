# GCache 缓存设计

## 缓存过期驱逐设计

### 优点

设计简单, 当访问到过期的元素，才将其从内存中驱逐, 类似 Redis 对过期元素处理方案

### 缺点

容易引发内存占用, 并且对于 LRU 算法, 容易驱逐常用元素，但却仍保留已过期的 key

## LFU 驱逐设计

当新增缓存时，若超出缓存容量时，会从最小使用的分区里进行驱逐元素

```go
type freqEntry struct {
	freq  uint
	items map[*lfuItem]struct{}
}
```

由于使用 Map 存储元素, 驱逐元素是随机进行, 会存在将最新缓存的数据被移除


改进: 若以数组存放数据，可以按照FIFO，优先将存放较久的缓存数据进行移除

```go
for item := range entry.Value.(*freqEntry).items {
    if i >= count {
        return
    }
    c.removeItem(item)
    i++
}
```

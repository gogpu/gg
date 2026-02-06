package cache

// lruNode is a node in a doubly-linked LRU list.
// The node stores a key for O(1) deletion from the parent map.
type lruNode[K comparable] struct {
	key  K
	prev *lruNode[K]
	next *lruNode[K]
}

// lruList is a doubly-linked list for LRU eviction.
// The list is not thread-safe; callers must handle synchronization.
//
// The head is the most recently used, tail is least recently used.
type lruList[K comparable] struct {
	head *lruNode[K]
	tail *lruNode[K]
	len  int
}

// newLRUList creates an empty LRU list.
func newLRUList[K comparable]() *lruList[K] {
	return &lruList[K]{}
}

// Len returns the number of nodes in the list.
func (l *lruList[K]) Len() int {
	return l.len
}

// PushFront adds a new node at the front (most recently used).
// Returns the created node for later access.
func (l *lruList[K]) PushFront(key K) *lruNode[K] {
	node := &lruNode[K]{key: key}
	if l.head == nil {
		// Empty list
		l.head = node
		l.tail = node
	} else {
		// Insert at front
		node.next = l.head
		l.head.prev = node
		l.head = node
	}
	l.len++
	return node
}

// MoveToFront moves an existing node to the front (most recently used).
func (l *lruList[K]) MoveToFront(node *lruNode[K]) {
	if node == nil || node == l.head {
		return
	}

	// Remove from current position
	l.unlink(node)

	// Insert at front
	node.prev = nil
	node.next = l.head
	if l.head != nil {
		l.head.prev = node
	}
	l.head = node
	if l.tail == nil {
		l.tail = node
	}
	l.len++
}

// Remove removes a node from the list.
func (l *lruList[K]) Remove(node *lruNode[K]) {
	if node == nil {
		return
	}
	l.unlink(node)
}

// RemoveOldest removes and returns the key of the least recently used node.
// Returns zero value and false if list is empty.
func (l *lruList[K]) RemoveOldest() (K, bool) {
	if l.tail == nil {
		var zero K
		return zero, false
	}

	node := l.tail
	l.unlink(node)
	return node.key, true
}

// Oldest returns the key of the least recently used node without removing it.
// Returns zero value and false if list is empty.
func (l *lruList[K]) Oldest() (K, bool) {
	if l.tail == nil {
		var zero K
		return zero, false
	}
	return l.tail.key, true
}

// Clear removes all nodes from the list.
func (l *lruList[K]) Clear() {
	l.head = nil
	l.tail = nil
	l.len = 0
}

// unlink removes a node from the list without clearing the node's pointers.
// Used internally by Remove and MoveToFront.
func (l *lruList[K]) unlink(node *lruNode[K]) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.head = node.next
	}

	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.tail = node.prev
	}

	node.prev = nil
	node.next = nil
	l.len--
}

package util

import (
	"sync"
)

// SafeSet 基于sync.Map实现的并发安全Set
type SafeSet[T comparable] struct {
	M sync.Map
}

func NewSafeSetFromSlice[T comparable](items []T) *SafeSet[T] {
	set := NewSafeSet[T]()
	for _, item := range items {
		set.Add(item)
	}
	return set
}

// NewSafeSet 创建一个新的并发安全Set
func NewSafeSet[T comparable]() *SafeSet[T] {
	return &SafeSet[T]{}
}

// Add 添加元素到Set
func (s *SafeSet[T]) Add(value T) {
	s.M.Store(value, struct{}{})
}

// Contains 检查元素是否存在于Set中
func (s *SafeSet[T]) Contains(value T) bool {
	_, ok := s.M.Load(value)
	return ok
}

// Remove 从Set中移除元素
func (s *SafeSet[T]) Remove(value T) {
	s.M.Delete(value)
}

// Size 获取Set中元素数量
func (s *SafeSet[T]) Size() int {
	count := 0
	s.M.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// Clear 清空Set
func (s *SafeSet[T]) Clear() {
	s.M.Range(func(key, _ interface{}) bool {
		s.M.Delete(key)
		return true
	})
}

// Values 获取Set中所有元素
func (s *SafeSet[T]) Values() []T {
	var values []T
	s.M.Range(func(key, _ interface{}) bool {
		values = append(values, key.(T))
		return true
	})
	return values
}

// Union 并集操作
func (s *SafeSet[T]) Union(other *SafeSet[T]) *SafeSet[T] {
	result := NewSafeSet[T]()

	// 添加当前Set的所有元素
	s.M.Range(func(key, _ interface{}) bool {
		result.Add(key.(T))
		return true
	})

	// 添加另一个Set的所有元素
	other.M.Range(func(key, _ interface{}) bool {
		result.Add(key.(T))
		return true
	})

	return result
}

// Intersection 交集操作
func (s *SafeSet[T]) Intersection(other *SafeSet[T]) *SafeSet[T] {
	result := NewSafeSet[T]()

	s.M.Range(func(key, _ interface{}) bool {
		if other.Contains(key.(T)) {
			result.Add(key.(T))
		}
		return true
	})

	return result
}

// Difference 差集操作 (s - other)
func (s *SafeSet[T]) Difference(other *SafeSet[T]) *SafeSet[T] {
	result := NewSafeSet[T]()

	s.M.Range(func(key, _ interface{}) bool {
		if !other.Contains(key.(T)) {
			result.Add(key.(T))
		}
		return true
	})

	return result
}

// 遍历Set并执行操作
func (s *SafeSet[T]) Range(f func(value T) bool) {
	s.M.Range(func(key, _ interface{}) bool {
		return f(key.(T))
	})
}

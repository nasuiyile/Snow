package util

//
//type ReentrantLock struct {
//	mu        sync.Mutex
//	owner     int64 // 持有锁的 Goroutine ID
//	recursion int   // 递归次数
//}
//
//func (r *ReentrantLock) Lock() {
//
//	gid := getGoroutineID()
//
//	if r.owner == gid {
//		r.recursion++
//		return
//	}
//
//	r.mu.Lock()
//	r.owner = gid
//	r.recursion = 1
//}
//
//func (r *ReentrantLock) Unlock() {
//	if r.owner != getGoroutineID() {
//		panic("unlocking from different goroutine")
//	}
//
//	r.recursion--
//	if r.recursion == 0 {
//		r.owner = -1
//		r.mu.Unlock()
//	}
//}
//
//func getGoroutineID() int64 {
//	var buf [64]byte
//	var s = buf[:runtime.Stack(buf[:], false)]
//	s = s[len("goroutine "):]
//	s = s[:bytes.IndexByte(s, ' ')]
//	gid, _ := strconv.ParseInt(string(s), 10, 64)
//	return gid
//}

/**
 * @Author: zhangchao
 * @Description:
 * @Date: 2022/11/18 5:34 PM
 */
package main

import (
	"sync"

	"google.golang.org/grpc"
)

type connectionPool struct {
	pool           map[string]*grpc.ClientConn
	referenceCount map[*grpc.ClientConn]int
	referenceLock  *sync.RWMutex
}

func newConnectionPool() *connectionPool {
	return &connectionPool{
		pool:           map[string]*grpc.ClientConn{},
		referenceCount: map[*grpc.ClientConn]int{},
		referenceLock:  &sync.RWMutex{},
	}
}

func (p *connectionPool) Register(address string, conn *grpc.ClientConn) {
	if oldConn, ok := p.pool[address]; ok {
		// oldConn is not used by pool anymore
		p.Release(oldConn)
	}

	p.pool[address] = conn
	// conn is used by caller and pool
	// NOTE: pool should also increment referenceCount not to close the pooled connection

	p.referenceLock.Lock()
	p.referenceCount[conn] = 2
	p.referenceLock.Unlock()
}

func (p *connectionPool) Share(address string) (*grpc.ClientConn, bool) {
	conn, ok := p.pool[address]
	if !ok {
		return nil, false
	}

	p.referenceLock.Lock()
	p.referenceCount[conn]++
	p.referenceLock.Unlock()
	return conn, true
}

func (p *connectionPool) Release(conn *grpc.ClientConn) {
	p.referenceLock.Lock()
	defer p.referenceLock.Unlock()

	if _, ok := p.referenceCount[conn]; !ok {
		return
	}

	p.referenceCount[conn]--

	// for concurrent use, connection is closed after all callers release it
	if p.referenceCount[conn] <= 0 {
		conn.Close()
		delete(p.referenceCount, conn)
	}
}

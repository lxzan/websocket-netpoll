package internal

import (
	"bytes"
	"sync"
)

type BufferPool struct {
	p0 sync.Pool
	p1 sync.Pool
	p2 sync.Pool
	p3 sync.Pool
}

func NewBufferPool() *BufferPool {
	var p = &BufferPool{
		p0: sync.Pool{},
		p1: sync.Pool{},
		p2: sync.Pool{},
		p3: sync.Pool{},
	}
	p.p0.New = func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, Lv1))
	}
	p.p1.New = func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, Lv2))
	}
	p.p2.New = func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, Lv3))
	}
	p.p3.New = func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, Lv4))
	}
	return p
}

func (p *BufferPool) Put(b *bytes.Buffer) {
	if b == nil {
		return
	}
	n := b.Cap()
	if n == 0 || n > Lv5 {
		return
	}

	b.Reset()
	if n <= Lv1 {
		p.p0.Put(b)
		return
	}
	if n <= Lv2 {
		p.p1.Put(b)
		return
	}
	if n <= Lv3 {
		p.p2.Put(b)
		return
	}
	if n <= Lv4 {
		p.p3.Put(b)
		return
	}
}

func (p *BufferPool) Get(n int) *bytes.Buffer {
	if n <= Lv1 {
		buf := p.p0.Get().(*bytes.Buffer)
		return buf
	}
	if n <= Lv2 {
		buf := p.p1.Get().(*bytes.Buffer)
		return buf
	}
	if n <= Lv3 {
		buf := p.p2.Get().(*bytes.Buffer)
		return buf
	}
	if n <= Lv4 {
		buf := p.p3.Get().(*bytes.Buffer)
		return buf
	}
	return bytes.NewBuffer(make([]byte, 0, n))
}

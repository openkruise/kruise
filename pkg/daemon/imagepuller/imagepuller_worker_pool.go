package imagepuller

import (
	"sync"

	"k8s.io/klog/v2"
)

type Task func()

type ImagePullWorkerPool interface {
	Submit(fn Task)
	Start()
	Stop()
}

type chanPool struct {
	queue    chan Task
	wg       sync.WaitGroup
	maxWorks int
}

func NewChanPool(n int) ImagePullWorkerPool {
	return &chanPool{
		queue:    make(chan Task, n),
		maxWorks: n,
	}
}

func (p *chanPool) Submit(task Task) {
	p.queue <- task
}

func (p *chanPool) Start() {
	for i := 0; i < p.maxWorks; i++ {
		go func() {
			defer p.wg.Done()
			for task := range p.queue {
				task()
			}
		}()
		p.wg.Add(1)
	}
}

func (p *chanPool) Stop() {
	close(p.queue)
	p.wg.Wait()
	klog.Info("all worker in image pull worker pool stopped")
}

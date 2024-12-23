package util

import (
	"k8s.io/client-go/tools/cache"
	toolscache "sigs.k8s.io/controller-runtime/pkg/cache"
)

// GetCacheIndexer helps to get the cache indexer from the informer
func GetCacheIndexer(informer toolscache.Informer) cache.Indexer {
	shardIndexInformer, ok := informer.(cache.SharedIndexInformer)
	if ok {
		return shardIndexInformer.GetIndexer()
	}
	indexers := cache.Indexers{}
	err := informer.(toolscache.Informer).AddIndexers(indexers)
	if err != nil {
		panic(err)
	}
	return cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, indexers)
}

// Central memory-only data storage location for persisting certain data between asynchronous client requests
package datastore

import "sync"

type storeType struct {
	mutex sync.RWMutex
	data  map[string]map[string]any // userID(jwt based username): dataID(uuid): raw data
}

var dataStore = storeType{
	data: make(map[string]map[string]any),
}

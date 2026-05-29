package datastore

import "fmt"

// Saves raw data into global memory
func Put(userID string, dataID string, data any) {
	dataStore.mutex.Lock()
	defer dataStore.mutex.Unlock()

	_, userMapInitialized := dataStore.data[userID]
	if !userMapInitialized {
		dataStore.data[userID] = make(map[string]any)
	}

	dataStore.data[userID][dataID] = data
}

// Retrieves raw data into global memory
func Get(userID string, dataID string) (data any, err error) {
	dataStore.mutex.RLock()
	defer dataStore.mutex.RUnlock()

	_, userMapInitialized := dataStore.data[userID]
	if !userMapInitialized {
		err = fmt.Errorf("user %s has no data stored", userID)
		return
	}

	data, dataPresent := dataStore.data[userID][dataID]
	if !dataPresent {
		err = fmt.Errorf("no data for id %s", userID)
		return
	}

	return
}

// Removes raw data into global memory
func Delete(userID string, dataID string) {
	dataStore.mutex.Lock()
	defer dataStore.mutex.Unlock()

	if userMap, ok := dataStore.data[userID]; ok {
		delete(userMap, dataID)

		// Remove user map when empty
		if len(userMap) == 0 {
			delete(dataStore.data, userID)
		}
	}
}

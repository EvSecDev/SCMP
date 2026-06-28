// Handles External DRN configurations (files)
package drnconfig

// Supplied DRN file configs
type CfgNode map[string]CfgValue
type CfgValue struct {
	kind uint8 // 0 = string, 1 = object
	str  string
	obj  CfgNode
}

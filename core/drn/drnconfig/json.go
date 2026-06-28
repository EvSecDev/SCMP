package drnconfig

import (
	"encoding/json"
	"fmt"
)

func (value *CfgValue) UnmarshalJSON(data []byte) (err error) {
	// Try string first
	var text string
	err = json.Unmarshal(data, &text)
	if err == nil {
		value.kind = kindString
		value.str = text
		value.obj = nil
		return
	}

	// Try object
	var obj CfgNode
	err = json.Unmarshal(data, &obj)
	if err == nil {
		if len(obj) == 0 {
			err = fmt.Errorf("empty objects are not permitted")
			return
		}

		value.kind = kindObject
		value.obj = obj
		value.str = ""
		return
	}

	err = fmt.Errorf("value must be string or object")
	return
}

func (value CfgValue) MarshalJSON() (data []byte, err error) {
	switch value.kind {
	case kindString:
		data, err = json.Marshal(value.str)
	case kindObject:
		data, err = json.Marshal(value.obj)
	default:
		err = fmt.Errorf("invalid DRN value kind: %d", value.kind)
	}
	return
}

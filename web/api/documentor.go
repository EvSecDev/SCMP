package api

import (
	"context"
	"reflect"
	"scmp/web/internal"
	"sort"
	"strings"
)

func handleAPIBrowser(baseCtx context.Context, clientCtx context.Context, fullReq internal.Request) (resp any, errObj internal.Error) {
	defs := []map[string]any{}

	for _, api := range internal.GetAPIDef() {
		if api.Method == "api.browser" {
			continue
		}

		def := map[string]any{
			"name":        api.Name,
			"description": api.Description,
			"method":      api.Method,
		}

		if api.Params != nil {
			def["params"] = generateSampleValue(api.Params)
		} else {
			def["params"] = nil
		}

		if api.Result != nil {
			def["result"] = generateSampleValue(api.Result)
		} else {
			def["result"] = nil
		}

		defs = append(defs, def)
	}

	sort.Slice(defs, func(a, b int) bool {
		ma, _ := defs[a]["method"].(string)
		mb, _ := defs[b]["method"].(string)
		return ma < mb
	})

	resp = defs
	return
}

func generateSampleValue(t reflect.Type) any {
	switch t.Kind() {
	case reflect.String:
		return "text"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return 123
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return 123
	case reflect.Float32, reflect.Float64:
		return 123.45
	case reflect.Bool:
		return true
	case reflect.Slice:
		elem := generateSampleValue(t.Elem())
		return []any{elem}
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			val := generateSampleValue(t.Elem())
			return map[string]any{"key": val}
		}
		return map[any]any{}
	case reflect.Ptr:
		return generateSampleValue(t.Elem())
	case reflect.Struct:
		sample := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)

			// Exported only
			if !field.IsExported() {
				continue
			}

			jsonKey := field.Name
			if tag := field.Tag.Get("json"); tag != "" {
				parts := strings.Split(tag, ",")
				if parts[0] != "" {
					jsonKey = parts[0]
				}
			}

			sample[jsonKey] = generateSampleValue(field.Type)
		}
		return sample
	default:
		return nil
	}
}

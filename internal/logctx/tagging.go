package logctx

import (
	"context"
)

// Append new tag to tag list.
// New context contains a copy of the old context tag slice to ensure new context slice is owned only by the new context.
func AppendCtxTag(ctx context.Context, newTag string) (newCtx context.Context) {
	oldTags := GetTagList(ctx)

	// Copy old slice, prevents mutation by parent context
	copiedTags := append(append([]string(nil), oldTags...), newTag)

	newCtx = context.WithValue(ctx, LogTagsKey, copiedTags)
	return
}

// Removes last index of tag list.
// New context contains a copy of the old context tag slice to ensure new context slice is owned only by the new context.
func RemoveLastCtxTag(ctx context.Context) (newCtx context.Context) {
	oldTags := GetTagList(ctx)

	// Copy old slice, prevents mutation by parent context
	copiedTags := append([]string(nil), oldTags...)

	if len(copiedTags) > 0 {
		copiedTags = copiedTags[:len(copiedTags)-1]
	}

	newCtx = context.WithValue(ctx, LogTagsKey, copiedTags)
	return
}

// Overwrites entire tag list with given list
// New context contains a copy of the old context tag slice to ensure new context slice is owned only by the new context.
func OverwriteCtxTag(ctx context.Context, newTags []string) (newCtx context.Context) {
	// Copy old slice, prevents mutation by parent context
	copiedTags := append([]string(nil), newTags...)

	newCtx = context.WithValue(ctx, LogTagsKey, copiedTags)
	return
}

// Extracts and copies tag list from context or returns empty array if no tags exist on context.
func GetTagList(ctx context.Context) (tagListCopy []string) {
	currentTags, ok := ctx.Value(LogTagsKey).([]string)
	if !ok {
		tagListCopy = []string{}
		return
	}

	// Copy old slice, prevents mutation of context list by manipulation of returned list
	tagListCopy = append([]string(nil), currentTags...)
	return
}

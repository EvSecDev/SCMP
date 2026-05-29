package logctx

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func ctxWithTags(tags []string) context.Context {
	return context.WithValue(context.Background(), LogTagsKey, tags)
}

func assertTags(t *testing.T, ctx context.Context, want []string) {
	t.Helper()
	got := GetTagList(ctx)
	if got == nil {
		got = []string{}
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tags mismatch: got=%v want=%v", got, want)
	}
}

func TestGetTagList(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want []string
	}{
		{
			name: "no value in context",
			ctx:  context.Background(),
			want: []string{},
		},
		{
			name: "correct slice stored",
			ctx:  ctxWithTags([]string{"a", "b"}),
			want: []string{"a", "b"},
		},
		{
			name: "wrong type stored",
			ctx:  context.WithValue(context.Background(), LogTagsKey, "nope"),
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertTags(t, tt.ctx, tt.want)
		})
	}
}

func TestGetTagList_ReturnedSliceIsIndependent(t *testing.T) {
	ctx := ctxWithTags([]string{"a", "b"})

	tags := GetTagList(ctx)
	tags[0] = "mutated"

	// Original context must remain unchanged
	assertTags(t, ctx, []string{"a", "b"})
}

func TestAppendCtxTag(t *testing.T) {
	tests := []struct {
		name      string
		startTags []string
		appendTag string
		want      []string
	}{
		{
			name:      "append to empty",
			startTags: []string{},
			appendTag: "a",
			want:      []string{"a"},
		},
		{
			name:      "append to existing",
			startTags: []string{"a", "b"},
			appendTag: "c",
			want:      []string{"a", "b", "c"},
		},
		{
			name:      "append empty string",
			startTags: []string{"a"},
			appendTag: "",
			want:      []string{"a", ""},
		},
		{
			name:      "append duplicate",
			startTags: []string{"a"},
			appendTag: "a",
			want:      []string{"a", "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := ctxWithTags(tt.startTags)
			newCtx := AppendCtxTag(orig, tt.appendTag)

			assertTags(t, newCtx, tt.want)
			assertTags(t, orig, tt.startTags) // immutability
		})
	}
}

func TestAppendCtxTag_CopyOnWrite(t *testing.T) {
	orig := ctxWithTags([]string{"a"})
	newCtx := AppendCtxTag(orig, "b")

	tags := GetTagList(newCtx)
	tags[0] = "mutated"

	assertTags(t, orig, []string{"a"})
	assertTags(t, newCtx, []string{"a", "b"})
}

func TestRemoveLastCtxTag(t *testing.T) {
	tests := []struct {
		name      string
		startTags []string
		want      []string
	}{
		{
			name:      "remove from empty",
			startTags: []string{},
			want:      []string{},
		},
		{
			name:      "remove single element",
			startTags: []string{"a"},
			want:      []string{},
		},
		{
			name:      "remove from multiple",
			startTags: []string{"a", "b", "c"},
			want:      []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := ctxWithTags(tt.startTags)
			newCtx := RemoveLastCtxTag(orig)

			assertTags(t, newCtx, tt.want)
			assertTags(t, orig, tt.startTags) // immutability
		})
	}
}

func TestRemoveLastCtxTag_RepeatedCalls(t *testing.T) {
	ctx := ctxWithTags([]string{"a"})

	ctx = RemoveLastCtxTag(ctx)
	assertTags(t, ctx, []string{})

	ctx = RemoveLastCtxTag(ctx)
	assertTags(t, ctx, []string{}) // no panic, still empty
}

func TestOverwriteCtxTag(t *testing.T) {
	tests := []struct {
		name      string
		startTags []string
		newTags   []string
		want      []string
	}{
		{
			name:      "overwrite empty with non-empty",
			startTags: []string{},
			newTags:   []string{"x"},
			want:      []string{"x"},
		},
		{
			name:      "overwrite non-empty with empty",
			startTags: []string{"a", "b"},
			newTags:   []string{},
			want:      []string{},
		},
		{
			name:      "overwrite replaces all tags",
			startTags: []string{"a", "b"},
			newTags:   []string{"x", "y"},
			want:      []string{"x", "y"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := ctxWithTags(tt.startTags)
			newCtx := OverwriteCtxTag(orig, tt.newTags)

			assertTags(t, newCtx, tt.want)
			assertTags(t, orig, tt.startTags) // parent unchanged
		})
	}
}

func TestOverwriteCtxTag_Immutability(t *testing.T) {
	newTags := []string{"a"}
	ctx := OverwriteCtxTag(context.Background(), newTags)

	newTags[0] = "mutated"

	assertTags(t, ctx, []string{"a"})
}

func TestCtxTag_Chaining(t *testing.T) {
	ctx := context.Background()

	ctx = AppendCtxTag(ctx, "a")
	ctx = AppendCtxTag(ctx, "b")
	ctx = RemoveLastCtxTag(ctx)
	ctx = AppendCtxTag(ctx, "c")

	assertTags(t, ctx, []string{"a", "c"})
}

func TestCtxTag_OverwriteThenAppend(t *testing.T) {
	ctx := ctxWithTags([]string{"a", "b"})
	ctx = OverwriteCtxTag(ctx, []string{"x"})
	ctx = AppendCtxTag(ctx, "y")

	assertTags(t, ctx, []string{"x", "y"})
}

func TestContextTags_ConcurrentImmutability(t *testing.T) {
	baseCtx := context.Background()
	baseCtx = OverwriteCtxTag(baseCtx, []string{"base"})

	const goroutines = 8

	type result struct {
		id   int
		tags []string
	}

	results := make(chan result, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(id int) {
			defer wg.Done()

			ctx := baseCtx

			// Each goroutine performs independent modifications
			ctx = AppendCtxTag(ctx, fmt.Sprintf("worker-%d", id))
			ctx = AppendCtxTag(ctx, "step1")
			ctx = RemoveLastCtxTag(ctx)
			ctx = AppendCtxTag(ctx, "final")

			results <- result{
				id:   id,
				tags: GetTagList(ctx),
			}
		}(i)
	}

	wg.Wait()
	close(results)

	// Parent context must be unchanged
	parentTags := GetTagList(baseCtx)
	if !reflect.DeepEqual(parentTags, []string{"base"}) {
		t.Fatalf("parent tags mutated: got=%v want=%v", parentTags, []string{"base"})
	}

	// Each goroutine must see only its own tags
	seen := make(map[int]bool)

	for res := range results {
		expected := []string{
			"base",
			fmt.Sprintf("worker-%d", res.id),
			"final",
		}

		if !reflect.DeepEqual(res.tags, expected) {
			t.Fatalf(
				"goroutine %d tags mismatch: got=%v want=%v",
				res.id,
				res.tags,
				expected,
			)
		}

		seen[res.id] = true
	}

	// Ensure all goroutines reported results
	if len(seen) != goroutines {
		t.Fatalf("expected %d goroutines, got %d", goroutines, len(seen))
	}
}

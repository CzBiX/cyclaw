package tool

import "context"

const subTaskDepthKey contextKey = "sub_task_depth"

// WithSubTaskDepth returns a new context carrying the given sub-task nesting depth.
func WithSubTaskDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, subTaskDepthKey, depth)
}

// SubTaskDepthFrom extracts the sub-task nesting depth from the context.
// Returns 0 if no depth has been set.
func SubTaskDepthFrom(ctx context.Context) int {
	d, _ := ctx.Value(subTaskDepthKey).(int)
	return d
}

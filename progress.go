package main

import "context"

type progressReporter func(string)

type progressReporterContextKey struct{}

func withProgressReporter(ctx context.Context, reporter progressReporter) context.Context {
	if reporter == nil {
		return ctx
	}
	return context.WithValue(ctx, progressReporterContextKey{}, reporter)
}

func reportProgress(ctx context.Context, message string) {
	reporter, _ := ctx.Value(progressReporterContextKey{}).(progressReporter)
	if reporter != nil {
		reporter(message)
	}
}

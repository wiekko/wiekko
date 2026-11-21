// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package templates

import "context"

type noticeKey struct{}

// WithNotice returns ctx carrying a notice for the page being rendered: a
// short message the app layout shows as a toast, for example why a form the
// user asked for was not shown (controller.WithNotice puts it there).
func WithNotice(ctx context.Context, message string) context.Context {
	return context.WithValue(ctx, noticeKey{}, message)
}

// NoticeFrom returns the notice WithNotice stored in ctx, or "".
func NoticeFrom(ctx context.Context) string {
	s, _ := ctx.Value(noticeKey{}).(string)
	return s
}

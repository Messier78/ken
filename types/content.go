package types

import "context"

type Content struct {
	Domain       string
	App          string
	Name         string
	KeyString    string
	DnzbRawQuery string
	RawQuery     string
	ClientAddr   string
	ServerAddr   string
	LocalAddr    string
	RemoteIP     string
	Referer      string
	PageURL      string
	SwfURL       string
	Scheme       string

	Status       int
	TimestampFix bool
}

type key int

var contKey key

func NewContext(ctx context.Context, cont *Content) context.Context {
	return context.WithValue(ctx, contKey, cont)
}

func FromContext(ctx context.Context) (*Content, bool) {
	c, ok := ctx.Value(contKey).(*Content)
	return c, ok
}

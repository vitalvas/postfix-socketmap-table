package socketmap

import "context"

type LookupFn func(ctx context.Context, key string) (*Result, error)

type DefaultLookupFn func(ctx context.Context, dict, key string) (*Result, error)

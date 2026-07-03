package inoculant

import (
	"context"
	"errors"

	"k8s.io/client-go/rest"
)

func Apply(ctx context.Context, dir string, cfg *rest.Config) error {
	return errors.New("not implemented")
}

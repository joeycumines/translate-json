package main

import (
	"context"
	"fmt"
	translate "github.com/dafanasev/go-yandex-translate"
	"strings"
)

type (
	Yandex struct {
		APIKey   string
		Language string
	}
)

func (x *Yandex) Translate(ctx context.Context, line, offset, length int, value string) (string, error) {
	if strings.TrimSpace(value) == `` {
		return value, nil
	}

	if x.APIKey == `` {
		return ``, fmt.Errorf(`yandex: requires api key`)
	}

	translation, err := translate.New(x.APIKey).Translate(x.Language, value)
	if err != nil {
		return ``, err
	}

	return translation.Result(), nil
}

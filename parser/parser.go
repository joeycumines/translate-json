package parser

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	commandName   = `translate-json`
	lineSeparator = "\n"
)

type (
	Translator interface {
		Translate(ctx context.Context, line, offset, length int, value string) (string, error)
	}

	Engine struct {
		mu    sync.RWMutex
		cache map[string]string
	}

	translateConfig struct {
		*translateState
		deferred      []func()
		translator    Translator
		scanner       *bufio.Scanner
		customScanner bool
		writer        io.Writer
		buffer        []byte
		noCopy        sync.Mutex
	}

	translateState struct {
		ctx    context.Context
		cancel context.CancelFunc
		line   []byte
		ok     bool
		number int
	}

	TranslateOption func(c *translateConfig) error
)

var (
	ErrTranslatorRequired = errors.New(commandName + `: translator required`)
	ErrInputRequired      = errors.New(commandName + `: input required`)
	ErrOutputRequired     = errors.New(commandName + `: output required`)
	ErrInvalidOption      = errors.New(commandName + `: invalid option`)
)

func WithTranslator(translator Translator) TranslateOption {
	return func(c *translateConfig) error {
		c.translator = translator
		return nil
	}
}

func WithInputScanner(scanner *bufio.Scanner) TranslateOption {
	return func(c *translateConfig) error {
		c.scanner = scanner
		c.customScanner = true
		return nil
	}
}

func WithInputFilePath(filePath string) TranslateOption {
	return func(c *translateConfig) error {
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		c.deferred = append(c.deferred, func() { _ = file.Close() })

		c.scanner = bufio.NewScanner(file)
		c.customScanner = false

		return nil
	}
}

func WithMaxLineLength(max int) TranslateOption {
	return func(c *translateConfig) error {
		if max <= 0 {
			return fmt.Errorf(`%w: max line length must be > 0`, ErrInvalidOption)
		}

		c.buffer = make([]byte, max)

		return nil
	}
}

func WithOutputFilePath(filePath string) TranslateOption {
	return func(c *translateConfig) error {
		file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		c.deferred = append(c.deferred, func() { _ = file.Close() })

		return WithOutputWriter(file)(c)
	}
}

func WithOutputWriter(writer io.Writer) TranslateOption {
	return func(c *translateConfig) error {
		c.writer = writer
		return nil
	}
}

func (x *Engine) Translate(ctx context.Context, options ...TranslateOption) error {
	c := translateConfig{translateState: &translateState{}}

	defer func() {
		for _, fn := range c.deferred {
			//goland:noinspection GoDeferInLoop
			defer fn()
		}
		c = translateConfig{}
	}()

	c.ctx, c.cancel = context.WithCancel(ctx)
	defer c.cancel()

	for _, option := range options {
		if err := option(&c); err != nil {
			return err
		}
	}

	if err := c.validate(); err != nil {
		return err
	}

	if err := c.finalise(); err != nil {
		return err
	}

	if err := c.validate(); err != nil {
		return err
	}

	return x.translate(&c)
}

func (x *Engine) translate(c *translateConfig) error {
	for c.next() {
		if line := bytes.TrimLeftFunc(c.line, jsonWhitespaceRuneFunc); len(line) != 0 && line[0] == '"' {
			var (
				offset    = len(c.line) - len(line)
				length    int
				value     string
				translate = func() error {
					// WARNING race-y caching

					x.mu.RLock()
					s, ok := x.cache[value]
					x.mu.RUnlock()

					var (
						b   []byte
						err error
					)
					if ok {
						b, err = json.Marshal(s)
					} else {
						s, err = c.translator.Translate(c.ctx, c.number, offset, length, value)
						if err == nil {
							b, err = json.Marshal(s)
							if err == nil {
								x.mu.Lock()
								if x.cache == nil {
									x.cache = make(map[string]string)
								}
								x.cache[value] = s
								x.mu.Unlock()
							}
						}
					}
					if err != nil {
						return err
					}

					if _, err := io.Copy(c.writer, bytes.NewReader(c.line[:offset])); err != nil {
						return err
					}
					if _, err := io.Copy(c.writer, bytes.NewReader(b)); err != nil {
						return err
					}
					if _, err := io.Copy(c.writer, bytes.NewReader(c.line[offset+length:])); err != nil {
						return err
					}
					if _, err := io.Copy(c.writer, strings.NewReader(lineSeparator)); err != nil {
						return err
					}
					return nil
				}
				reader  = bytes.NewReader(line)
				decoder = json.NewDecoder(reader)
				parse   = func() error {
					if err := decoder.Decode(&value); err != nil {
						return newInputError(err)
					}
					line = line[len(line)-decoder.Buffered().(*bytes.Reader).Len()-reader.Len():]
					length = len(c.line) - len(line) - offset
					line = bytes.TrimLeftFunc(line, jsonWhitespaceRuneFunc)
					return nil
				}
			)
			if err := parse(); err != nil {
				return err
			}
			if len(line) == 0 {
				// probably the end of an array, maybe a string by itself
				if err := translate(); err != nil {
					return err
				}
				continue
			}
			switch line[0] {
			case ',':
				if len(bytes.TrimLeftFunc(line[1:], jsonWhitespaceRuneFunc)) == 0 {
					// probably an array element, with more following
					if err := translate(); err != nil {
						return err
					}
					continue
				}
			case ':':
				line = bytes.TrimLeftFunc(line[1:], jsonWhitespaceRuneFunc)
				if len(line) != 0 && line[0] == '"' {
					offset = len(c.line) - len(line)
					reader = bytes.NewReader(line)
					decoder = json.NewDecoder(reader)
					if err := parse(); err != nil {
						return err
					}
					if len(line) == 0 {
						// probably the last element in an object
						if err := translate(); err != nil {
							return err
						}
						continue
					}
					if line[0] == ',' && len(bytes.TrimLeftFunc(line[1:], jsonWhitespaceRuneFunc)) == 0 {
						// probably an object element, with more following
						if err := translate(); err != nil {
							return err
						}
						continue
					}
				}
			}
		}
		// fallback is to write out as-is
		if _, err := io.Copy(c.writer, bytes.NewReader(c.line)); err != nil {
			return err
		}
		if _, err := io.Copy(c.writer, strings.NewReader(lineSeparator)); err != nil {
			return err
		}
	}
	return nil
}

func (x *translateConfig) validate() error {
	if x.translator == nil {
		return ErrTranslatorRequired
	}

	if x.scanner == nil {
		return ErrInputRequired
	}

	if x.writer == nil {
		return ErrOutputRequired
	}

	return nil
}

func (x *translateConfig) finalise() error {
	if n := len(x.buffer); n != 0 {
		if x.customScanner {
			return fmt.Errorf(`%w: custom scanner incompatible with max line length option`, ErrInvalidOption)
		}
		x.scanner.Buffer(x.buffer, n)
	}

	return nil
}

func (x *translateConfig) next() bool {
	x.number++
	if !x.scanner.Scan() {
		x.line, x.ok = nil, false
	} else {
		x.line, x.ok = x.scanner.Bytes(), true
	}
	return x.ok
}

func jsonWhitespaceRuneFunc(r rune) bool {
	switch r {
	case '\u0020',
		'\u000A',
		'\u000D',
		'\u0009':
		return true
	}
	return false
}

func newInputError(err error) error {
	return fmt.Errorf(commandName+`: input error: %w`, err)
}

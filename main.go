package main

import (
	"context"
	"fmt"
	"github.com/joeycumines/translate-json/parser"
	"github.com/urfave/cli/v2"
	"log"
	"os"
)

const (
	translatorYandex = `yandex`
	languageEnglish  = `en`
)

type (
	Command struct {
		Config struct {
			Translator   string
			Language     string
			YandexAPIKey string
		}
	}
)

func main() {
	command := Command{}

	app := cli.App{
		Name:      "translate-json",
		Usage:     "text diffable machine translation of json files",
		UsageText: "translate-json [global options] input-file output-file",
		Flags:     command.Flags(),
		Action:    command.Action,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func (x *Command) Flags() (flags []cli.Flag) {
	flags = append(
		flags,
		&cli.StringFlag{
			Name:        `translator`,
			Usage:       `chose the translator to use (currently only "` + translatorYandex + `")`,
			EnvVars:     []string{`APP_TRANSLATOR`},
			Value:       translatorYandex,
			Destination: &x.Config.Translator,
		},
		&cli.StringFlag{
			Name:        `language`,
			Usage:       `language to translate to`,
			EnvVars:     []string{`APP_LANGUAGE`},
			Value:       languageEnglish,
			Destination: &x.Config.Language,
		},
		&cli.StringFlag{
			Name:        `yandex-api-key`,
			Usage:       `api key for yandex (required for "` + translatorYandex + `" translator)`,
			EnvVars:     []string{`APP_YANDEX_API_KEY`},
			Destination: &x.Config.YandexAPIKey,
		},
	)

	return
}

func (x *Command) Action(c *cli.Context) error {
	if n := c.NArg(); n != 2 {
		return fmt.Errorf(`invalid number of args: %d`, n)
	}

	inputFile := c.Args().Get(0)
	if inputFile == `` {
		return fmt.Errorf(`invalid input file: ""`)
	}

	outputFile := c.Args().Get(1)
	if outputFile == `` {
		return fmt.Errorf(`invalid output file: ""`)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var translator parser.Translator
	switch x.Config.Translator {
	case translatorYandex:
		if x.Config.YandexAPIKey == `` {
			return fmt.Errorf(`empty or missing yandex-api-key`)
		}

		translator = &Yandex{
			APIKey:   x.Config.YandexAPIKey,
			Language: x.Config.Language,
		}

	default:
		return fmt.Errorf(`invalid translator: %s`, x.Config.Translator)
	}

	return new(parser.Engine).Translate(
		ctx,
		parser.WithInputFilePath(inputFile),
		parser.WithOutputFilePath(outputFile),
		parser.WithTranslator(translator),
	)
}

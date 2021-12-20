package parser

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"testing"
)

func loadTestResource(name string) []byte {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to find caller source")
	}
	file, err := os.Open(path.Join(path.Dir(source), `testdata`, name))
	if err != nil {
		panic(err)
	}
	defer file.Close()
	b, err := ioutil.ReadAll(file)
	if err != nil {
		panic(err)
	}
	return b
}

func storeTestResource(name string, b []byte) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		panic("failed to find caller source")
	}
	file, err := os.OpenFile(path.Join(path.Dir(source), `testdata`, name), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if _, err := io.Copy(file, bytes.NewReader(b)); err != nil {
		panic(err)
	}
	if err := file.Close(); err != nil {
		panic(err)
	}
}

type mapTranslator map[string]string

func (m mapTranslator) Translate(ctx context.Context, line, offset, length int, value string) (string, error) {
	if line <= 0 {
		panic(line)
	}
	if offset < 0 {
		panic(offset)
	}
	if length < 0 {
		panic(length)
	}
	if v, ok := m[value]; ok {
		return v, nil
	}
	return value, nil
}

func TestEngine_Translate_inputOutput(t *testing.T) {
	for _, tc := range [...]struct {
		Name    string
		Input   string
		Output  string
		Options []TranslateOption
		Err     error
	}{
		{
			Name:   `partial dashboard`,
			Input:  `translate-01-input.json`,
			Output: `translate-01-output.json`,
			Options: []TranslateOption{
				WithTranslator(mapTranslator{
					`【中文版本】2021.10.10更新，kubernetes资源全面展示！包含K8S整体资源总览、微服务资源明细、Pod资源明细及K8S网络带宽，优化重要指标展示。https://github.com/starsliao/Prometheus`: `[Chinese version] Update 2021.10.10, full display of kubernetes resources! Including K8S overall resource overview, microservice resource details, Pod resource details and K8S network bandwidth, and optimize the display of important indicators. https://github.com/starsliao/Prometheus`,
					`查看更多仪表板`: `View more dashboards`,
					"pls!":    "no",
					"https://github.com/starsliao/Prometheus": "abcdeasd",
					"":            "mmm",
					"object:1091": "",
				}),
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			var (
				scanner = bufio.NewScanner(bytes.NewReader(loadTestResource(tc.Input)))
				output  bytes.Buffer
				engine  Engine
			)
			err := engine.Translate(context.Background(), append([]TranslateOption{
				WithInputScanner(scanner),
				WithOutputWriter(&output),
			}, tc.Options...)...)
			if (err == nil) != (tc.Err == nil) || (err != nil && err.Error() != tc.Err.Error()) {
				t.Error(err)
			}
			if output := output.String(); output != string(loadTestResource(tc.Output)) {
				//storeTestResource(tc.Output, []byte(output))
				t.Error(output)
			}
		})
	}
}

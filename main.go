package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/cucumber/gherkin-go"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type config struct {
	dry    bool
	indent int
	align  string
}

func fmtFile(file string, cfg *config) error {
	stat, err := os.Stat(file)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return nil
	}
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("could not open %q: %+v", file, err)
	}
	gherkinDocument, err := gherkin.ParseGherkinDocument(f)
	if err != nil {
		f.Close()
		return fmt.Errorf("could not open %q: %+v", file, err)
	}
	f.Close()
	if gherkinDocument.Feature == nil {
		return fmt.Errorf("empty feature body")
	}
	var result bytes.Buffer
	write := func(indent int, f string, args ...interface{}) {
		add := strings.Repeat(" ", indent*cfg.indent)
		lines := strings.Split(fmt.Sprintf(f, args...), "\n")
		for _, line := range lines {
			result.WriteString(add + line + "\n")
		}
	}
	write(0, "Feature: %s", gherkinDocument.Feature.Name)
	write(0, gherkinDocument.Feature.Description)
	write(0, "")

	for _, c := range gherkinDocument.Feature.Children {

		fmtString := func(v *gherkin.DocString) {
			defer write(2, "\"\"\"")
			write(2, "\"\"\"")

			var a interface{}
			err := json.Unmarshal([]byte(v.Content), &a)
			if err != nil {
				write(0, v.Content)
				return
			}
			var buf bytes.Buffer
			e := json.NewEncoder(&buf)
			e.SetEscapeHTML(false)
			e.SetIndent("", strings.Repeat(" ", cfg.indent))
			if err = e.Encode(a); err != nil {
				write(0, v.Content)
				return
			}
			write(2, strings.TrimSpace(buf.String()))
		}

		fmtTable := func(v *gherkin.DataTable) {
			align := make([]int, len(v.Rows[0].Cells))
			sanitize := func(val string) string {
				val = strings.Replace(val, "|", "\\|", -1)
				return val
			}
			for i := range v.Rows {
				for j, col := range v.Rows[i].Cells {
					align[j] = max(align[j], len(sanitize(col.Value)))
				}
			}
			format := "|"
			for _, a := range align {
				switch cfg.align {
				case "right":
					format += " %" + strconv.Itoa(a) + "s |"
				case "left":
					format += " %-" + strconv.Itoa(a) + "s |"
				}
			}
			for i := range v.Rows {
				args := make([]interface{}, len(v.Rows[i].Cells))
				for j, col := range v.Rows[i].Cells {
					args[j] = sanitize(col.Value)
				}
				write(3, format, args...)
			}
		}

		var steps []*gherkin.Step
		var examples []*gherkin.DataTable
		switch v := c.(type) {
		case *gherkin.Background:
			if v.Name != "" {
				write(1, "Background: %s", strings.TrimSpace(v.Name))
			} else {
				write(1, "Background:")
			}
			steps = v.Steps
		case *gherkin.Scenario:
			write(1, "Scenario: %s", strings.TrimSpace(v.Name))
			steps = v.Steps
		case *gherkin.ScenarioOutline:
			write(1, "Scenario Outline: %s", strings.TrimSpace(v.Name))
			steps = v.Steps
			examples = make([]*gherkin.DataTable, len(v.Examples))
			for i, ex := range v.Examples {
				examples[i] = &gherkin.DataTable{
					Rows: append([]*gherkin.TableRow{ex.TableHeader}, ex.TableBody...),
				}
			}
		default:
			return fmt.Errorf("unhandled feature children: %T", v)
		}

		for _, step := range steps {
			def := strings.Replace(step.Keyword+" "+step.Text, "  ", " ", -1)
			write(2, "%s", def)
			if step.Argument == nil {
				continue
			}
			switch v := step.Argument.(type) {
			case *gherkin.DocString:
				fmtString(v)
				continue
			case *gherkin.DataTable:
				fmtTable(v)
				continue
			default:
				return fmt.Errorf("unsupported step argument: %T\n", v)
			}
		}

		for _, ex := range examples {
			write(0, "")
			write(2, "Examples:")
			fmtTable(ex)
		}

		write(0, "")
	}

	if cfg.dry {
		fmt.Println(strings.TrimSpace(result.String()))
		return nil
	}

	return ioutil.WriteFile(file, bytes.TrimSpace(result.Bytes()), 666)
}

func main() {
	var (
		dry    = flag.Bool("dry", false, "run in dry mode")
		indent = flag.Int("indent", 2, "amount of whitespaces for indentation")
		align  = flag.String("align", "left", "align tables left|right")
	)
	flag.Parse()

	for i := 0; i < flag.NArg(); i++ {
		name := flag.Arg(i)
		if err := fmtFile(name, &config{
			dry:    *dry,
			indent: *indent,
			align:  *align,
		}); err != nil {
			fmt.Printf("skip %s: %+v\n", name, err)
			continue
		}
		fmt.Println(name)
	}
}

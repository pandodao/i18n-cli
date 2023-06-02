package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pandodao/i18n-cli/cmd/parser"
	"github.com/pandodao/i18n-cli/internal/gpt"

	"github.com/spf13/cobra"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

var translateCmd = &cobra.Command{
	Use: "translate",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			fmt.Println("environment variable OPENAI_API_KEY is empty")
			return
		}

		gptHandler := gpt.New(gpt.Config{
			Keys:    []string{apiKey},
			Timeout: time.Duration(10) * time.Second,
		})

		source, others, indep, err := provideFiles(cmd)
		if err != nil {
			cmd.PrintErrln("read files failed")
			return
		}

		cmd.Printf("📝 source: %d records\n", len(source.LocaleItemsMap))
		cmd.Println("🌐 Generating locale files:")

		for _, item := range others {
			process(ctx, gptHandler, source, item, indep)
		}
	},
}

func process(ctx context.Context, gptHandler *gpt.Handler, source *parser.LocaleFileContent, target *parser.LocaleFileContent, indep *parser.LocaleFileContent) error {
	count := 1
	for k, v := range source.LocaleItemsMap {
		needToTranslate := false
		if len(v) != 0 {
			if _, ok := target.LocaleItemsMap[k]; !ok {
				// key does not exist, translate it
				needToTranslate = true
			} else {
				// key exists
				if indep != nil {
					if v, found := indep.LocaleItemsMap[k]; found {
						// key is in independent map, use the value in independent map
						target.LocaleItemsMap[k] = v
					}
				} else if strings.EqualFold(target.LocaleItemsMap[k], v) || len(target.LocaleItemsMap[k]) == 0 {
					// same value or empty string, translate it
					needToTranslate = true
				} else if target.LocaleItemsMap[k][0] == '!' {
					// value starts with "!", translate it
					needToTranslate = true
				}
			}

			if needToTranslate {
				result, err := gptHandler.Translate(ctx, v, target.Lang)
				if err != nil {
					return err
				}
				target.LocaleItemsMap[k] = result
			}

			fmt.Printf("\r🔄 %s: %d/%d", target.Path, count, len(source.LocaleItemsMap))
			count += 1
		}
	}

	buf, err := target.JSON()
	if err != nil {
		return err
	}

	err = os.WriteFile(target.Path, buf, 0644)
	if err != nil {
		return err
	}

	fmt.Printf("\r✅ %s: %d/%d\n", target.Path, len(source.LocaleItemsMap), len(source.LocaleItemsMap))

	return nil
}

func provideFiles(cmd *cobra.Command) (source *parser.LocaleFileContent, others []*parser.LocaleFileContent, indep *parser.LocaleFileContent, err error) {

	indepFile, err := cmd.Flags().GetString("independent")
	if err != nil {
		return
	}
	if indepFile != "" {
		indep = &parser.LocaleFileContent{}
		if err = indep.ParseFromJSONFile(indepFile); err != nil {
			return
		}
	}

	sourceFile, err := cmd.Flags().GetString("source")
	if err != nil {
		return
	}
	if sourceFile != "" {
		source = &parser.LocaleFileContent{}
		if err = source.ParseFromJSONFile(sourceFile); err != nil {
			return
		}

		var lang string
		lang, err = langCodeToName("en-US")
		if err != nil {
			return
		}

		source.Code = "en-US"
		source.Lang = lang
	} else {
		err = fmt.Errorf("source file is required")
		return
	}

	dir, err := cmd.Flags().GetString("dir")
	if err != nil {
		return
	}
	if dir != "" {
		others = make([]*parser.LocaleFileContent, 0)
		items, _ := os.ReadDir(dir)
		sourceBaseFile := filepath.Base(sourceFile)
		for _, item := range items {
			if !item.IsDir() {
				name := filepath.Base(item.Name())
				ext := filepath.Ext(name)
				if strings.EqualFold(item.Name(), sourceBaseFile) {
					continue
				}

				if strings.ToLower(ext) != ".json" {
					fmt.Printf("file %s is not a JSON file. skip this file.\n", name)
					continue
				}

				localeContent := &parser.LocaleFileContent{}
				if err = localeContent.ParseFromJSONFile(path.Join(dir, item.Name())); err != nil {
					fmt.Println("parse file failed: ", err, ". skip this file.")
					continue
				}

				others = append(others, localeContent)
			}
		}
	} else {
		err = fmt.Errorf("dir is required")
		return
	}

	return
}

func langCodeToName(code string) (string, error) {
	tag, err := language.Parse(code)
	if err != nil {
		return "", err
	}
	return display.Self.Name(tag), nil
}

func init() {
	translateCmd.Flags().String("dir", "", "the directory of language files")
	translateCmd.Flags().String("source", "", "the source language file")
	translateCmd.Flags().String("independent", "", "the independent language file")

	rootCmd.AddCommand(translateCmd)
}

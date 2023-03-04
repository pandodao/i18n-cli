package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"i18n-cli/internal/gpt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

type LocaleContent struct {
	Code    string
	Lang    string
	Path    string
	Content map[string]string
}

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

		sourceContent, contentArray, err := provideFiles(cmd)
		if err != nil {
			cmd.PrintErrln("read files failed")
			return
		}

		cmd.Printf("📝 source: %d records\n", len(sourceContent.Content))
		cmd.Println("🌐 Generating locale files:")

		for _, item := range contentArray {
			process(ctx, gptHandler, sourceContent, item)
		}
	},
}

func process(ctx context.Context, gptHandler *gpt.Handler, source *LocaleContent, target *LocaleContent) error {
	count := 1
	for k, v := range source.Content {
		needToTranslate := false
		if _, ok := target.Content[k]; !ok {
			// key does not exist, translate it
			needToTranslate = true
		} else {
			// key exists
			if strings.EqualFold(target.Content[k], v) || len(target.Content[k]) == 0 {
				// same value or empty string, translate it
				needToTranslate = true
			} else if target.Content[k][0] == '!' {
				// value starts with "!", translate it
				needToTranslate = true
			}
		}

		if needToTranslate {
			result, err := gptHandler.Translate(ctx, v, target.Lang)
			if err != nil {
				return err
			}
			target.Content[k] = result
		}

		fmt.Printf("\r🔄 %s: %d/%d", target.Path, count, len(source.Content))
		count += 1
	}

	// write to file
	targetBytes, err := json.MarshalIndent(target.Content, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(target.Path, targetBytes, 0644)
	if err != nil {
		return err
	}

	fmt.Printf("\r✅ %s: %d/%d\n", target.Path, len(source.Content), len(source.Content))

	return nil
}

func provideFiles(cmd *cobra.Command) (*LocaleContent, []*LocaleContent, error) {
	dir, err := cmd.Flags().GetString("dir")
	if err != nil {
		return nil, nil, err
	}

	sourceFileName, err := cmd.Flags().GetString("source")
	if err != nil {
		return nil, nil, err
	}

	sourcePath := path.Join(dir, sourceFileName)

	if _, err := os.Stat(sourcePath); err != nil {
		return nil, nil, err
	}

	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, nil, err
	}

	lang, err := langCodeToName("en-US")
	if err != nil {
		return nil, nil, err
	}
	source := &LocaleContent{
		Code:    "en-US",
		Lang:    lang,
		Path:    sourcePath,
		Content: make(map[string]string),
	}
	json.Unmarshal(sourceBytes, &source.Content)

	localeContents := make([]*LocaleContent, 0)
	// read the json files
	items, _ := os.ReadDir(dir)
	for _, item := range items {
		if !item.IsDir() {
			name := filepath.Base(item.Name()) // get base name of file
			ext := filepath.Ext(name)          // get extension
			nameWithoutExt := name[0 : len(name)-len(ext)]

			if strings.EqualFold(name, sourceFileName) {
				continue
			}

			if strings.ToLower(ext) != ".json" {
				continue
			}

			lang, err := langCodeToName(nameWithoutExt)
			if err != nil {
				cmd.PrintErrf("failed to get language: %+v\n", name)
				continue
			}

			localeContent := &LocaleContent{
				Code:    nameWithoutExt,
				Lang:    lang,
				Path:    path.Join(dir, name),
				Content: make(map[string]string),
			}

			fileBytes, err := os.ReadFile(path.Join(dir, name))
			if err != nil {
				return nil, nil, err
			}
			json.Unmarshal(fileBytes, &localeContent.Content)

			localeContents = append(localeContents, localeContent)
		}
	}

	return source, localeContents, nil
}

func langCodeToName(code string) (string, error) {
	tag, err := language.Parse(code)
	if err != nil {
		return "", err
	}
	return display.Self.Name(tag), nil
}

func init() {
	translateCmd.Flags().String("dir", "locales", "the directory of language files")
	translateCmd.Flags().String("source", "en-US.json", "the source language file")

	rootCmd.AddCommand(translateCmd)
}

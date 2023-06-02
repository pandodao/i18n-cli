package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

type LocaleFileContent struct {
	Code string
	Lang string
	Path string

	LocaleItemsMap map[string]string
}

func (l *LocaleFileContent) ParseFromJSONFile(path string) error {
	var err error
	if _, err = os.Stat(path); err != nil {
		return err
	}

	name := filepath.Base(path) // get base name of file
	ext := filepath.Ext(name)   // get extension
	nameWithoutExt := name[0 : len(name)-len(ext)]

	if strings.ToLower(ext) != ".json" {
		return fmt.Errorf("file %s is not a json file", name)
	}

	lang, err := langCodeToName(nameWithoutExt)
	if err != nil {
		return err
	}

	l.Code = nameWithoutExt
	l.Lang = lang
	l.Path = path

	if l.LocaleItemsMap == nil {
		l.LocaleItemsMap = make(map[string]string)
	}

	// read the json file
	sourceBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// convert
	var data map[string]interface{}
	if err := json.Unmarshal(sourceBytes, &data); err != nil {
		return err
	}
	result := make(map[string]string)
	flatten(data, "", result)

	l.LocaleItemsMap = result
	return nil
}

func (l *LocaleFileContent) JSON() ([]byte, error) {
	nestedData := nestedInsertion(l.LocaleItemsMap)
	sortedData := sortMapKeys(nestedData)

	jsonData, err := json.MarshalIndent(sortedData, "", "  ")
	if err != nil {
		return nil, err
	}
	return jsonData, nil
}

func flatten(input map[string]interface{}, currentKey string, result map[string]string) {
	for key, value := range input {
		newKey := key
		if currentKey != "" {
			newKey = currentKey + "/" + key
		}
		switch child := value.(type) {
		case map[string]interface{}:
			flatten(child, newKey, result)
		default:
			result[newKey] = fmt.Sprint(value)
		}
	}
}

func nestedInsertion(input map[string]string) map[string]interface{} {
	data := make(map[string]interface{})
	for key, value := range input {
		parts := strings.Split(key, "/")
		currentMap := data
		for i, part := range parts {
			if i == len(parts)-1 {
				currentMap[part] = value
			} else {
				if _, exist := currentMap[part]; !exist {
					currentMap[part] = make(map[string]interface{})
				}
				currentMap = currentMap[part].(map[string]interface{})
			}
		}
	}
	return data
}

func sortMapKeys(data interface{}) interface{} {
	switch data := data.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(data))
		for key := range data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make(map[string]interface{}, len(data))
		for _, key := range keys {
			result[key] = sortMapKeys(data[key])
		}
		return result
	default:
		return data
	}
}

func langCodeToName(code string) (string, error) {
	tag, err := language.Parse(code)
	if err != nil {
		return "", err
	}
	return display.Self.Name(tag), nil
}

package chatimage

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

const (
	promptSourceID   = "banana-prompt-quicker"
	promptSourceName = "Banana Prompt Quicker"
	promptAssetBase  = "https://glidea.github.io"
)

//go:embed default_prompt_library.json
var bundledPromptLibrary []byte

type promptLibraryItem struct {
	ID                 string   `json:"id"`
	SourceID           string   `json:"source_id"`
	SourceName         string   `json:"source_name"`
	Title              string   `json:"title"`
	Prompt             string   `json:"prompt"`
	Description        string   `json:"description"`
	Preview            string   `json:"preview"`
	Link               string   `json:"link"`
	Author             string   `json:"author"`
	Category           string   `json:"category"`
	SubCategory        string   `json:"sub_category"`
	Tags               []string `json:"tags"`
	ReferenceImageURLs []string `json:"reference_image_urls"`
	ImageMode          string   `json:"image_mode"`
	ImageModel         string   `json:"image_model"`
	ImageSize          string   `json:"image_size"`
	ImageCount         *int     `json:"image_count"`
}

type promptLibrarySource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Homepage    string `json:"homepage"`
	Enabled     bool   `json:"enabled"`
	PromptCount int    `json:"prompt_count"`
}

type promptLibraryView struct {
	SchemaVersion      int                   `json:"schema_version"`
	Version            string                `json:"version"`
	PromptCount        int                   `json:"prompt_count"`
	SourceCount        int                   `json:"source_count"`
	EnabledSourceCount int                   `json:"enabled_source_count"`
	Items              []promptLibraryItem   `json:"items"`
	Sources            []promptLibrarySource `json:"sources"`
}

type bundledPromptFile struct {
	Version string              `json:"version"`
	Prompts []bundledPromptItem `json:"prompts"`
}

type bundledPromptItem struct {
	ID                 string          `json:"id"`
	Title              string          `json:"title"`
	Prompt             string          `json:"prompt"`
	Description        string          `json:"description"`
	Preview            string          `json:"preview"`
	Link               string          `json:"link"`
	Author             string          `json:"author"`
	Category           string          `json:"category"`
	SubCategory        string          `json:"sub_category"`
	Tags               []string        `json:"tags"`
	ReferenceImageURLs []string        `json:"reference_image_urls"`
	Mode               string          `json:"mode"`
	ImageModel         string          `json:"image_model"`
	ImageSize          string          `json:"image_size"`
	ImageCount         json.RawMessage `json:"image_count"`
}

var (
	promptLibraryOnce sync.Once
	promptLibraryData *promptLibraryView
	promptLibraryErr  error
)

func loadPromptLibrary() (*promptLibraryView, error) {
	promptLibraryOnce.Do(func() {
		var bundled bundledPromptFile
		if err := json.Unmarshal(bundledPromptLibrary, &bundled); err != nil {
			promptLibraryErr = fmt.Errorf("decode bundled prompt library: %w", err)
			return
		}
		items := make([]promptLibraryItem, 0, len(bundled.Prompts))
		for _, raw := range bundled.Prompts {
			title, prompt := strings.TrimSpace(raw.Title), strings.TrimSpace(raw.Prompt)
			if title == "" || prompt == "" {
				continue
			}
			item := promptLibraryItem{
				ID:                 promptSourceID + ":" + strings.TrimSpace(raw.ID),
				SourceID:           promptSourceID,
				SourceName:         promptSourceName,
				Title:              title,
				Prompt:             prompt,
				Description:        strings.TrimSpace(raw.Description),
				Preview:            normalizePromptAssetURL(raw.Preview),
				Link:               strings.TrimSpace(raw.Link),
				Author:             strings.TrimSpace(raw.Author),
				Category:           strings.TrimSpace(raw.Category),
				SubCategory:        strings.TrimSpace(raw.SubCategory),
				Tags:               nonNilStrings(raw.Tags),
				ReferenceImageURLs: normalizePromptAssetURLs(raw.ReferenceImageURLs),
				ImageMode:          normalizePromptMode(raw.Mode),
				ImageModel:         strings.TrimSpace(raw.ImageModel),
				ImageSize:          strings.TrimSpace(raw.ImageSize),
				ImageCount:         parsePromptImageCount(raw.ImageCount),
			}
			items = append(items, item)
		}
		promptLibraryData = &promptLibraryView{
			SchemaVersion:      1,
			Version:            bundled.Version,
			PromptCount:        len(items),
			SourceCount:        1,
			EnabledSourceCount: 1,
			Items:              items,
			Sources: []promptLibrarySource{{
				ID: promptSourceID, Name: promptSourceName,
				Homepage: "https://glidea.github.io/banana-prompt-quicker/",
				Enabled:  true, PromptCount: len(items),
			}},
		}
	})
	return promptLibraryData, promptLibraryErr
}

func PromptLibrary() (any, error) { return loadPromptLibrary() }

func normalizePromptMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "generate", "edit":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizePromptAssetURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return promptAssetBase + value
	}
	if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
		return value
	}
	return ""
}

func normalizePromptAssetURLs(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := normalizePromptAssetURL(value); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func parsePromptImageCount(raw json.RawMessage) *int {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var count int
	if json.Unmarshal(raw, &count) != nil {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return nil
		}
		count, _ = strconv.Atoi(strings.TrimSpace(text))
	}
	if count < 1 || count > 4 {
		return nil
	}
	return &count
}

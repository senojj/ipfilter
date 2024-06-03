package config

import (
	"encoding/json"
	"os"
)

type Settings struct {
	ArchiveURL     string   `json:"archive_url"`
	FileSuffixList []string `json:"file_suffix_list"`
	RefreshSeconds int      `json:"refresh_seconds"`
}

func Load(path string) (s *Settings, err error) {
	var f *os.File
	f, err = os.Open(path)
	if err != nil {
		return
	}
	err = json.NewDecoder(f).Decode(&s)
	return
}

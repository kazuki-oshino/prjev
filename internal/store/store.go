package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kazuki-oshino/prjev/internal/app"
	"github.com/kazuki-oshino/prjev/internal/model"
)

type Record struct {
	RecordSchemaVersion int          `json:"record_schema_version"`
	Evidence            app.Evidence `json:"evidence"`
	Result              model.Result `json:"result"`
}

func Save(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return save(path, append(b, '\n'))
}
func save(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("既存ファイルは上書きしません: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(dir, ".pradar-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Link(name, path)
}
func Read(path string) (Record, error) {
	var r Record
	b, e := os.ReadFile(path)
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(b, &r); e != nil {
		return r, e
	}
	if r.RecordSchemaVersion != 1 || r.Result.SchemaVersion != 1 || r.Evidence.Rules == nil {
		return r, fmt.Errorf("記録形式が不正です")
	}
	if r.Evidence.ReviewPolicyVersion < 0 || r.Evidence.ReviewPolicyVersion > 2 {
		return r, fmt.Errorf("対応していない判定方針のversionです")
	}
	return r, nil
}

func SaveBytes(path string, b []byte) error { return save(path, b) }

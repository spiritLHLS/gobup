package services

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gobup/server/internal/agent"
	"github.com/gobup/server/internal/database"
)

func CheckRecordedFilesFromConfig(limit int) (*agent.FileCheckResult, error) {
	return CheckRecordedFilesFromRequest(agent.FileCheckRequest{Limit: limit})
}

func CheckRecordedFilesFromRequest(req agent.FileCheckRequest) (*agent.FileCheckResult, error) {
	config := LoadConfigFromDB()
	paths := cleanScanPaths(req.Paths)
	if len(paths) == 0 {
		paths = getCustomScanPaths()
		if strings.TrimSpace(config.WorkPath) != "" {
			paths = append(paths, config.WorkPath)
		}
	}

	minSize := config.MinFileSize
	if req.MinSize != nil {
		minSize = *req.MinSize
	}
	extensions := config.VideoExtensions
	if len(req.Extensions) > 0 {
		extensions = req.Extensions
	}
	return CheckRecordedFiles(paths, minSize, req.Limit, extensions)
}

func CheckRecordedFiles(paths []string, minSize int64, limit int, extensions []string) (*agent.FileCheckResult, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if minSize < 0 {
		minSize = 0
	}
	extSet := make(map[string]struct{}, len(extensions))
	for _, ext := range extensions {
		ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
		if ext != "" {
			extSet[ext] = struct{}{}
		}
	}
	if len(extSet) == 0 {
		for _, ext := range []string{"flv", "mp4", "mkv", "ts"} {
			extSet[ext] = struct{}{}
		}
	}

	result := &agent.FileCheckResult{
		SampleLimit: limit,
		Files:       make([]agent.FileCheckItem, 0, limit),
		Errors:      make([]string, 0),
	}
	seen := make(map[string]struct{})
	allPaths := make([]string, 0)

	for _, rawPath := range paths {
		root := strings.TrimSpace(rawPath)
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			result.Errors = append(result.Errors, "路径不可用: "+root+" ("+err.Error()+")")
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				result.Errors = append(result.Errors, path+": "+err.Error())
				return nil
			}
			if info == nil || info.IsDir() {
				return nil
			}
			if _, ok := seen[path]; ok {
				return nil
			}
			seen[path] = struct{}{}
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
			if _, ok := extSet[ext]; !ok {
				return nil
			}
			if isDanmakuBurnOutput(filepath.Base(path)) || info.Size() < minSize {
				return nil
			}
			result.TotalFiles++
			result.TotalSize += info.Size()
			allPaths = append(allPaths, path)
			if len(result.Files) < limit {
				result.Files = append(result.Files, agent.FileCheckItem{
					FilePath: path,
					FileName: filepath.Base(path),
					FileSize: info.Size(),
					ModTime:  info.ModTime(),
				})
			}
			return nil
		})
		if err != nil {
			result.Errors = append(result.Errors, root+": "+err.Error())
		}
	}

	inDatabase := existingRecordedFileSet(allPaths)
	result.DatabaseAware = database.GetDB() != nil
	result.InDatabaseCount = len(inDatabase)
	result.NewFiles = result.TotalFiles - result.InDatabaseCount
	for i := range result.Files {
		inDB := inDatabase[result.Files[i].FilePath]
		result.Files[i].InDatabase = &inDB
	}
	result.WorkPath = strings.Join(paths, ",")
	return result, nil
}

func cleanScanPaths(rawPaths []string) []string {
	paths := make([]string, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		path := strings.TrimSpace(rawPath)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func existingRecordedFileSet(paths []string) map[string]bool {
	existing := make(map[string]bool)
	if len(paths) == 0 || database.GetDB() == nil {
		return existing
	}
	db := database.GetDB()
	const batchSize = 500
	for start := 0; start < len(paths); start += batchSize {
		end := start + batchSize
		if end > len(paths) {
			end = len(paths)
		}
		var rows []string
		db.Table("record_history_parts AS p").
			Select("p.file_path").
			Joins("JOIN record_histories h ON h.id = p.history_id AND h.deleted_at IS NULL").
			Where("p.file_path IN ?", paths[start:end]).
			Scan(&rows)
		for _, path := range rows {
			existing[path] = true
		}
	}
	return existing
}

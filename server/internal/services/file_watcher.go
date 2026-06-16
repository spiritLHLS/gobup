package services

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
)

const (
	fileWatcherDebounce = 90 * time.Second
	fileWatcherRefresh  = 5 * time.Minute
)

type FileWatcherService struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
}

func NewFileWatcherService() *FileWatcherService {
	return &FileWatcherService{}
}

func (s *FileWatcherService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cancel = cancel
	s.done = done

	go s.run(ctx, done)
}

func (s *FileWatcherService) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()

	if cancel == nil {
		return
	}

	cancel()
	if done == nil {
		return
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Println("[FileWatcher] 停止等待超时")
	}
}

func (s *FileWatcherService) run(ctx context.Context, done chan struct{}) {
	defer close(done)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[FileWatcher] 创建监听器失败: %v", err)
		return
	}
	defer watcher.Close()

	scanSvc := NewFileScanService()
	watchedDirs := make(map[string]struct{})
	triggerScan := make(chan struct{}, 1)
	refreshTicker := time.NewTicker(fileWatcherRefresh)
	defer refreshTicker.Stop()

	addConfiguredDirs := func() {
		config := LoadConfigFromDB()
		paths := append([]string{}, getCustomScanPaths()...)
		if config.WorkPath != "" {
			paths = append(paths, config.WorkPath)
		}
		for _, path := range paths {
			s.addWatchTree(watcher, watchedDirs, path)
		}
	}

	addConfiguredDirs()
	log.Println("[FileWatcher] 文件事件监听已启动")

	var scanTimer *time.Timer
	var scanTimerC <-chan time.Time
	defer func() {
		if scanTimer != nil {
			scanTimer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Println("[FileWatcher] 文件事件监听已停止")
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if s.handleEvent(watcher, watchedDirs, scanSvc, event) {
				select {
				case triggerScan <- struct{}{}:
				default:
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[FileWatcher] 监听错误: %v", err)
		case <-triggerScan:
			if scanTimer == nil {
				scanTimer = time.NewTimer(fileWatcherDebounce)
			} else {
				if !scanTimer.Stop() {
					select {
					case <-scanTimer.C:
					default:
					}
				}
				scanTimer.Reset(fileWatcherDebounce)
			}
			scanTimerC = scanTimer.C
		case <-scanTimerC:
			scanTimerC = nil
			s.scanIfEnabled(scanSvc)
		case <-refreshTicker.C:
			addConfiguredDirs()
		}
	}
}

func (s *FileWatcherService) handleEvent(watcher *fsnotify.Watcher, watchedDirs map[string]struct{}, scanSvc *FileScanService, event fsnotify.Event) bool {
	if !isFileWatcherRelevantOp(event.Op) {
		return false
	}

	if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
		s.addWatchTree(watcher, watchedDirs, event.Name)
		return false
	}

	if !isFileWatcherVideoChangeEvent(scanSvc, event) {
		return false
	}

	log.Printf("[FileWatcher] 检测到录制文件变化，准备延迟扫盘: %s", event.Name)
	return true
}

func isFileWatcherRelevantOp(op fsnotify.Op) bool {
	return op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0
}

func isFileWatcherVideoChangeEvent(scanSvc *FileScanService, event fsnotify.Event) bool {
	if !isFileWatcherRelevantOp(event.Op) {
		return false
	}
	if scanSvc == nil {
		scanSvc = NewFileScanService()
	}
	ext := strings.ToLower(filepath.Ext(event.Name))
	return scanSvc.isVideoFile(ext, DefaultScanConfig("").VideoExtensions)
}

func (s *FileWatcherService) addWatchTree(watcher *fsnotify.Watcher, watchedDirs map[string]struct{}, root string) {
	if root == "" {
		return
	}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		if err != nil {
			log.Printf("[FileWatcher] 监听目录不可用，跳过: %s, error=%v", root, err)
		}
		return
	}

	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Printf("[FileWatcher] 访问目录失败，跳过: %s, error=%v", path, err)
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if _, exists := watchedDirs[path]; exists {
			return nil
		}
		if err := watcher.Add(path); err != nil {
			log.Printf("[FileWatcher] 添加监听目录失败: %s, error=%v", path, err)
			return nil
		}
		watchedDirs[path] = struct{}{}
		log.Printf("[FileWatcher] 已监听目录: %s", path)
		return nil
	}); err != nil {
		log.Printf("[FileWatcher] 遍历监听目录失败: %s, error=%v", root, err)
	}
}

func (s *FileWatcherService) scanIfEnabled(scanSvc *FileScanService) {
	db := database.GetDB()
	var config models.SystemConfig
	if err := db.First(&config).Error; err != nil {
		log.Printf("[FileWatcher] 获取系统配置失败，跳过事件扫盘: %v", err)
		return
	}
	if !config.AutoFileScan || !config.EnableFileWatcher {
		return
	}

	result, err := scanSvc.ScanAndImport(LoadConfigFromDB())
	if err != nil {
		if err == ErrScanAlreadyRunning {
			log.Println("[FileWatcher] 扫盘已在运行，本次事件触发跳过")
		} else {
			log.Printf("[FileWatcher] 事件触发扫盘失败: %v", err)
		}
		return
	}
	if result.NewFiles > 0 || result.FailedFiles > 0 {
		log.Printf("[FileWatcher] 事件触发扫盘完成: 总文件=%d, 新导入=%d, 跳过=%d, 失败=%d",
			result.TotalFiles, result.NewFiles, result.SkippedFiles, result.FailedFiles)
	}
}

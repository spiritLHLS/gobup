package services

import (
	"fmt"
	"log"
	"time"

	"github.com/gobup/server/internal/database"
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

// DataRepairService 数据修复服务，用于修复历史记录和分P之间的数据不一致问题
type DataRepairService struct{}

func NewDataRepairService() *DataRepairService {
	return &DataRepairService{}
}

// RepairResult 修复结果
type RepairResult struct {
	OrphanParts           int      `json:"orphanParts"`           // 孤儿分P（有分P但无历史记录）
	EmptyHistories        int      `json:"emptyHistories"`        // 空历史记录（有历史记录但无分P）
	CreatedHistories      int      `json:"createdHistories"`      // 新创建的历史记录
	DeletedEmptyHistories int      `json:"deletedEmptyHistories"` // 删除的空历史记录
	UpdatedHistoryTimes   int      `json:"updatedHistoryTimes"`   // 更新的历史记录时间
	ReassignedParts       int      `json:"reassignedParts"`       // 重新分配的分P
	Errors                []string `json:"errors"`                // 错误列表
}

// CheckAndRepairDataConsistency 检查并修复数据一致性问题
func (s *DataRepairService) CheckAndRepairDataConsistency(dryRun bool) (*RepairResult, error) {
	result := &RepairResult{
		Errors: make([]string, 0),
	}

	log.Printf("[DataRepair] 开始数据一致性检查 (dryRun=%v)", dryRun)

	// 1. 修复孤儿分P（有分P但无对应的历史记录）
	if err := s.repairOrphanParts(result, dryRun); err != nil {
		return result, fmt.Errorf("修复孤儿分P失败: %w", err)
	}

	// 2. 处理空历史记录（有历史记录但无分P）
	if err := s.handleEmptyHistories(result, dryRun); err != nil {
		return result, fmt.Errorf("处理空历史记录失败: %w", err)
	}

	// 3. 修复历史记录的时间范围（确保与分P时间一致）
	if err := s.repairHistoryTimeRanges(result, dryRun); err != nil {
		return result, fmt.Errorf("修复历史记录时间失败: %w", err)
	}

	log.Printf("[DataRepair] 数据一致性检查完成: 孤儿分P=%d, 空历史=%d, 新建历史=%d, 删除空历史=%d, 更新时间=%d",
		result.OrphanParts, result.EmptyHistories, result.CreatedHistories,
		result.DeletedEmptyHistories, result.UpdatedHistoryTimes)

	return result, nil
}

// repairOrphanParts 修复孤儿分P（有分P但无对应的历史记录）
func (s *DataRepairService) repairOrphanParts(result *RepairResult, dryRun bool) error {
	db := database.GetDB()

	// 查找所有孤儿分P（history_id 指向的记录不存在）
	var orphanParts []models.RecordHistoryPart
	err := db.Raw(`
		SELECT p.* 
		FROM record_history_parts p 
		LEFT JOIN record_histories h ON p.history_id = h.id 
		WHERE h.id IS NULL
	`).Scan(&orphanParts).Error

	if err != nil {
		return fmt.Errorf("查询孤儿分P失败: %w", err)
	}

	result.OrphanParts = len(orphanParts)

	if len(orphanParts) == 0 {
		log.Printf("[DataRepair] 未发现孤儿分P")
		return nil
	}

	log.Printf("[DataRepair] 发现 %d 个孤儿分P，准备修复", len(orphanParts))

	// 按 session_id 分组孤儿分P
	sessionGroups := make(map[string][]models.RecordHistoryPart)
	for _, part := range orphanParts {
		sessionGroups[part.SessionID] = append(sessionGroups[part.SessionID], part)
	}

	// 为每个session创建或查找历史记录
	for sessionID, parts := range sessionGroups {
		if len(parts) == 0 {
			continue
		}

		firstPart := parts[0]

		// 尝试查找同一session的历史记录
		var history models.RecordHistory
		err := db.Where("session_id = ?", sessionID).First(&history).Error

		if err == gorm.ErrRecordNotFound {
			// 历史记录不存在，创建新的
			if !dryRun {
				history = s.createHistoryFromPart(&firstPart, parts)
				if err := db.Create(&history).Error; err != nil {
					errMsg := fmt.Sprintf("为session %s 创建历史记录失败: %v", sessionID, err)
					result.Errors = append(result.Errors, errMsg)
					log.Printf("[DataRepair] %s", errMsg)
					continue
				}
				result.CreatedHistories++
				log.Printf("[DataRepair] 为session %s 创建了新的历史记录 (ID=%d)", sessionID, history.ID)
			} else {
				log.Printf("[DataRepair] [DryRun] 将为session %s 创建新的历史记录", sessionID)
				result.CreatedHistories++
			}
		} else if err != nil {
			errMsg := fmt.Sprintf("查询session %s 的历史记录失败: %v", sessionID, err)
			result.Errors = append(result.Errors, errMsg)
			log.Printf("[DataRepair] %s", errMsg)
			continue
		}

		// 将孤儿分P重新分配给历史记录
		if !dryRun && history.ID > 0 {
			for _, part := range parts {
				if err := db.Model(&part).Update("history_id", history.ID).Error; err != nil {
					errMsg := fmt.Sprintf("更新分P %d 的history_id失败: %v", part.ID, err)
					result.Errors = append(result.Errors, errMsg)
					log.Printf("[DataRepair] %s", errMsg)
				} else {
					result.ReassignedParts++
				}
			}
			log.Printf("[DataRepair] 将 %d 个孤儿分P重新分配给历史记录 %d", len(parts), history.ID)
		} else if dryRun {
			log.Printf("[DataRepair] [DryRun] 将把 %d 个孤儿分P重新分配给历史记录", len(parts))
			result.ReassignedParts += len(parts)
		}
	}

	return nil
}

// handleEmptyHistories 处理空历史记录（有历史记录但无分P）
func (s *DataRepairService) handleEmptyHistories(result *RepairResult, dryRun bool) error {
	db := database.GetDB()

	// 查找所有没有分P的历史记录
	var emptyHistories []models.RecordHistory
	err := db.Raw(`
		SELECT h.* 
		FROM record_histories h 
		LEFT JOIN record_history_parts p ON h.id = p.history_id 
		GROUP BY h.id 
		HAVING COUNT(p.id) = 0
	`).Scan(&emptyHistories).Error

	if err != nil {
		return fmt.Errorf("查询空历史记录失败: %w", err)
	}

	result.EmptyHistories = len(emptyHistories)

	if len(emptyHistories) == 0 {
		log.Printf("[DataRepair] 未发现空历史记录")
		return nil
	}

	log.Printf("[DataRepair] 发现 %d 个空历史记录", len(emptyHistories))

	// 删除超过30天的空历史记录
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	for _, history := range emptyHistories {
		if history.CreatedAt.Before(thirtyDaysAgo) {
			if !dryRun {
				if err := db.Delete(&history).Error; err != nil {
					errMsg := fmt.Sprintf("删除空历史记录 %d 失败: %v", history.ID, err)
					result.Errors = append(result.Errors, errMsg)
					log.Printf("[DataRepair] %s", errMsg)
				} else {
					result.DeletedEmptyHistories++
					log.Printf("[DataRepair] 删除了超过30天的空历史记录: ID=%d, SessionID=%s, 创建时间=%s",
						history.ID, history.SessionID, history.CreatedAt.Format("2006-01-02"))
				}
			} else {
				log.Printf("[DataRepair] [DryRun] 将删除空历史记录: ID=%d, SessionID=%s, 创建时间=%s",
					history.ID, history.SessionID, history.CreatedAt.Format("2006-01-02"))
				result.DeletedEmptyHistories++
			}
		} else {
			log.Printf("[DataRepair] 保留最近的空历史记录: ID=%d, SessionID=%s, 创建时间=%s (不到30天)",
				history.ID, history.SessionID, history.CreatedAt.Format("2006-01-02"))
		}
	}

	return nil
}

// repairHistoryTimeRanges 修复历史记录的时间范围
func (s *DataRepairService) repairHistoryTimeRanges(result *RepairResult, dryRun bool) error {
	db := database.GetDB()

	// 获取所有有分P的历史记录
	var histories []models.RecordHistory
	if err := db.Find(&histories).Error; err != nil {
		return fmt.Errorf("查询历史记录失败: %w", err)
	}

	for _, history := range histories {
		// 获取该历史记录的所有分P
		var parts []models.RecordHistoryPart
		if err := db.Where("history_id = ?", history.ID).
			Order("start_time ASC").
			Find(&parts).Error; err != nil {
			continue
		}

		if len(parts) == 0 {
			continue
		}

		// 计算正确的时间范围
		minStartTime := parts[0].StartTime
		maxEndTime := parts[0].EndTime

		for _, part := range parts {
			if part.StartTime.Before(minStartTime) {
				minStartTime = part.StartTime
			}
			if part.EndTime.After(maxEndTime) {
				maxEndTime = part.EndTime
			}
		}

		// 检查是否需要更新
		needUpdate := false
		if !history.StartTime.Equal(minStartTime) || !history.EndTime.Equal(maxEndTime) {
			needUpdate = true
		}

		if needUpdate {
			if !dryRun {
				if err := db.Model(&history).Updates(map[string]interface{}{
					"start_time": minStartTime,
					"end_time":   maxEndTime,
				}).Error; err != nil {
					errMsg := fmt.Sprintf("更新历史记录 %d 的时间范围失败: %v", history.ID, err)
					result.Errors = append(result.Errors, errMsg)
					log.Printf("[DataRepair] %s", errMsg)
				} else {
					result.UpdatedHistoryTimes++
					log.Printf("[DataRepair] 更新历史记录 %d 的时间范围: %s ~ %s",
						history.ID,
						minStartTime.Format("2006-01-02 15:04:05"),
						maxEndTime.Format("2006-01-02 15:04:05"))
				}
			} else {
				log.Printf("[DataRepair] [DryRun] 将更新历史记录 %d 的时间范围", history.ID)
				result.UpdatedHistoryTimes++
			}
		}
	}

	return nil
}

// createHistoryFromPart 从分P创建历史记录
func (s *DataRepairService) createHistoryFromPart(firstPart *models.RecordHistoryPart, allParts []models.RecordHistoryPart) models.RecordHistory {
	// 计算时间范围
	startTime := firstPart.StartTime
	endTime := firstPart.EndTime

	for _, part := range allParts {
		if part.StartTime.Before(startTime) {
			startTime = part.StartTime
		}
		if part.EndTime.After(endTime) {
			endTime = part.EndTime
		}
	}

	// 查找房间配置
	db := database.GetDB()
	var room models.RecordRoom
	uploadEnabled := true
	if err := db.Where("room_id = ?", firstPart.RoomID).First(&room).Error; err == nil {
		uploadEnabled = room.Upload
	}

	return models.RecordHistory{
		RoomID:    firstPart.RoomID,
		SessionID: firstPart.SessionID,
		EventID:   fmt.Sprintf("repair_%s_%d", firstPart.RoomID, time.Now().Unix()),
		Uname:     "未知主播",
		Title:     firstPart.LiveTitle,
		AreaName:  firstPart.AreaName,
		StartTime: startTime,
		EndTime:   endTime,
		Recording: false,
		Streaming: false,
		Upload:    uploadEnabled,
		Publish:   false,
	}
}

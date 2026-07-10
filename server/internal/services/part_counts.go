package services

import (
	"github.com/gobup/server/internal/models"
	"gorm.io/gorm"
)

type PublishablePartCounts struct {
	Uploaded  int64
	Total     int64
	Recording int64
}

type groupedPartCountRow struct {
	HistoryID uint
	Count     int64
}

func LoadPublishablePartCounts(db *gorm.DB, historyIDs []uint) map[uint]PublishablePartCounts {
	counts := make(map[uint]PublishablePartCounts, len(historyIDs))
	if db == nil || len(historyIDs) == 0 {
		return counts
	}
	for _, id := range historyIDs {
		counts[id] = PublishablePartCounts{}
	}

	mergeRows := func(rows []groupedPartCountRow, apply func(PublishablePartCounts, int64) PublishablePartCounts) {
		for _, row := range rows {
			current := counts[row.HistoryID]
			counts[row.HistoryID] = apply(current, row.Count)
		}
	}

	var uploadedRows []groupedPartCountRow
	db.Model(&models.RecordHistoryPart{}).
		Select("history_id, COUNT(*) AS count").
		Where("history_id IN ? AND upload = ? AND is_temp_file = ?", historyIDs, true, false).
		Group("history_id").
		Scan(&uploadedRows)
	mergeRows(uploadedRows, func(current PublishablePartCounts, count int64) PublishablePartCounts {
		current.Uploaded = count
		return current
	})

	var totalRows []groupedPartCountRow
	db.Model(&models.RecordHistoryPart{}).
		Select("history_id, COUNT(*) AS count").
		Where("history_id IN ? AND is_temp_file = ? AND NOT (file_delete = true AND upload = false)", historyIDs, false).
		Group("history_id").
		Scan(&totalRows)
	mergeRows(totalRows, func(current PublishablePartCounts, count int64) PublishablePartCounts {
		current.Total = count
		return current
	})

	var splitTotalRows []groupedPartCountRow
	db.Model(&models.RecordHistoryPart{}).
		Select("history_id, COUNT(*) AS count").
		Where("history_id IN ? AND is_temp_file = ? AND temp_file_type = ?", historyIDs, true, "split").
		Group("history_id").
		Scan(&splitTotalRows)
	var splitUploadedRows []groupedPartCountRow
	db.Model(&models.RecordHistoryPart{}).
		Select("history_id, COUNT(*) AS count").
		Where("history_id IN ? AND is_temp_file = ? AND temp_file_type = ? AND upload = ?", historyIDs, true, "split", true).
		Group("history_id").
		Scan(&splitUploadedRows)

	splitTotalByHistory := make(map[uint]int64, len(splitTotalRows))
	for _, row := range splitTotalRows {
		splitTotalByHistory[row.HistoryID] = row.Count
	}
	splitUploadedByHistory := make(map[uint]int64, len(splitUploadedRows))
	for _, row := range splitUploadedRows {
		splitUploadedByHistory[row.HistoryID] = row.Count
	}
	for historyID, current := range counts {
		if current.Total == 0 {
			current.Total = splitTotalByHistory[historyID]
			current.Uploaded = splitUploadedByHistory[historyID]
			counts[historyID] = current
		}
	}

	var recordingRows []groupedPartCountRow
	db.Model(&models.RecordHistoryPart{}).
		Select("history_id, COUNT(*) AS count").
		Where("history_id IN ? AND recording = ?", historyIDs, true).
		Group("history_id").
		Scan(&recordingRows)
	mergeRows(recordingRows, func(current PublishablePartCounts, count int64) PublishablePartCounts {
		current.Recording = count
		return current
	})

	return counts
}

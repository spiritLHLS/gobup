package controllers

// swaggerDocRoomCreate documents room creation.
//
// @Summary Add room
// @Description Adds a recording room.
// @Tags rooms
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /room/add [post]
func swaggerDocRoomCreate() {}

// swaggerDocRoomUpdate documents room updates.
//
// @Summary Update room
// @Description Updates room settings.
// @Tags rooms
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /room/update [post]
func swaggerDocRoomUpdate() {}

// swaggerDocRoomDelete documents room deletion.
//
// @Summary Delete room
// @Description Deletes a room by internal ID.
// @Tags rooms
// @Security BasicAuth
// @Produce json
// @Param id path int true "Room ID"
// @Success 200 {object} map[string]interface{}
// @Router /room/delete/{id} [get]
func swaggerDocRoomDelete() {}

// swaggerDocRoomUtilityRoutes documents room helper endpoints.
//
// @Summary Room helper APIs
// @Description Lists upload lines, recommended lines, line tests, seasons, and template verification results.
// @Tags rooms
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /room/lines [get]
// @Router /room/recommendedLines [get]
// @Router /room/testLines [get]
// @Router /room/testSpeed [get]
// @Router /room/seasons [get]
// @Router /room/verification [get]
// @Router /room/verifyTemplate [post]
func swaggerDocRoomUtilityRoutes() {}

// swaggerDocUserRoutes documents Bilibili account management.
//
// @Summary Bilibili user APIs
// @Description Lists users and manages login, refresh, status checks, deletion, updates, and enable state.
// @Tags users
// @Security BasicAuth
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} map[string]interface{}
// @Router /biliUser/list [get]
// @Router /biliUser/loginByCookie [post]
// @Router /biliUser/refresh/{id} [get]
// @Router /biliUser/checkStatus/{id} [get]
// @Router /biliUser/login [get]
// @Router /biliUser/loginCheck [get]
// @Router /biliUser/loginCancel [get]
// @Router /biliUser/delete/{id} [get]
// @Router /biliUser/update [post]
// @Router /biliUser/enabled/{id} [post]
func swaggerDocUserRoutes() {}

// swaggerDocHistoryEntityRoutes documents single-history operations.
//
// @Summary History entity APIs
// @Description Updates, deletes, uploads, publishes, syncs, and inspects a single recording history.
// @Tags histories
// @Security BasicAuth
// @Accept json
// @Produce json
// @Param id path int true "History ID"
// @Success 200 {object} map[string]interface{}
// @Router /history/export [post]
// @Router /history/update [post]
// @Router /history/delete/{id} [get]
// @Router /history/deleteWithFiles/{id} [post]
// @Router /history/resetStatus/{id} [post]
// @Router /history/upload/{id} [post]
// @Router /history/publish/{id} [post]
// @Router /history/updatePublishStatus/{id} [get]
// @Router /history/manualSetPublish/{id} [post]
// @Router /history/part/{id} [get]
// @Router /history/forceArchive/{id} [post]
// @Router /history/candidateFiles/{id} [get]
// @Router /history/sendDanmaku/{id} [post]
// @Router /history/danmakuStats/{id} [get]
// @Router /history/parseDanmaku/{id} [post]
// @Router /history/moveFiles/{id} [post]
// @Router /history/syncVideo/{id} [post]
// @Router /history/createSyncTask/{id} [post]
func swaggerDocHistoryEntityRoutes() {}

// swaggerDocHistoryBatchRoutes documents batch history operations.
//
// @Summary History batch APIs
// @Description Runs batch status, upload, publish, danmaku, move, sync, delete, and cleanup operations.
// @Tags histories
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /history/batchUpdate [post]
// @Router /history/batchDelete [post]
// @Router /history/batchUpload [post]
// @Router /history/batchPublish [post]
// @Router /history/batchResetStatus [post]
// @Router /history/batchDeleteWithFiles [post]
// @Router /history/cleanOld [post]
// @Router /history/batchSendDanmaku [post]
// @Router /history/batchParseDanmaku [post]
// @Router /history/batchMoveFiles [post]
// @Router /history/batchSyncVideo [post]
func swaggerDocHistoryBatchRoutes() {}

// swaggerDocSyncTaskRoutes documents video sync task APIs.
//
// @Summary Sync task APIs
// @Description Lists sync tasks and retries failed tasks.
// @Tags sync
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /syncTasks/list [get]
// @Router /syncTasks/retryFailed [post]
func swaggerDocSyncTaskRoutes() {}

// swaggerDocPartRoutes documents part APIs.
//
// @Summary Part APIs
// @Description Lists parts and uploads a part to editor.
// @Tags parts
// @Security BasicAuth
// @Accept json
// @Produce json
// @Param id path int true "History or part ID"
// @Success 200 {object} map[string]interface{}
// @Router /part/list/{id} [post]
// @Router /part/uploadEditor/{id} [get]
func swaggerDocPartRoutes() {}

// swaggerDocRateLimitRoutes documents upload rate limit APIs.
//
// @Summary Rate limit APIs
// @Description Reads and updates upload rate limit settings.
// @Tags ratelimit
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /ratelimit/config [get]
// @Router /ratelimit/config [post]
func swaggerDocRateLimitRoutes() {}

// swaggerDocCaptchaRoutes documents captcha APIs.
//
// @Summary Captcha APIs
// @Description Reads, submits, and clears captcha state.
// @Tags captcha
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /captcha/status [get]
// @Router /captcha/submit [post]
// @Router /captcha/clear [post]
func swaggerDocCaptchaRoutes() {}

// swaggerDocQueueControlRoutes documents upload queue control APIs.
//
// @Summary Upload queue controls
// @Description Pauses, resumes, cancels, and retries individual or pending upload tasks.
// @Tags queue
// @Security BasicAuth
// @Accept json
// @Produce json
// @Param id path int true "Part ID"
// @Success 200 {object} map[string]interface{}
// @Router /queue/upload/part/{id}/pause [post]
// @Router /queue/upload/part/{id}/resume [post]
// @Router /queue/upload/part/{id}/cancel [post]
// @Router /queue/upload/part/{id}/retry [post]
// @Router /queue/upload/pauseAll [post]
// @Router /queue/upload/resumeAll [post]
// @Router /queue/upload/cancelAll [post]
func swaggerDocQueueControlRoutes() {}

// swaggerDocQueueStatusRoutes documents other queue status APIs.
//
// @Summary Queue status APIs
// @Description Returns danmaku and parse queue status.
// @Tags queue
// @Security BasicAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /queue/danmaku/status [get]
// @Router /queue/parse/status [get]
func swaggerDocQueueStatusRoutes() {}

// swaggerDocProgressDanmaku documents danmaku progress.
//
// @Summary Get danmaku progress
// @Description Returns danmaku sending or burn-in progress for a history.
// @Tags progress
// @Security BasicAuth
// @Produce json
// @Param historyId path int true "History ID"
// @Success 200 {object} map[string]interface{}
// @Router /progress/danmaku/{historyId} [get]
func swaggerDocProgressDanmaku() {}

// swaggerDocConfigRoutes documents config import/export and system settings.
//
// @Summary Config APIs
// @Description Exports, imports, reads, updates, toggles, summarizes, and cleans up system configuration.
// @Tags config
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /config/export [post]
// @Router /config/import [post]
// @Router /config/system [get]
// @Router /config/system [put]
// @Router /config/toggle [post]
// @Router /config/stats [get]
// @Router /config/cleanup [post]
func swaggerDocConfigRoutes() {}

// swaggerDocLogRoutes documents log APIs.
//
// @Summary Log APIs
// @Description Reads and clears server logs.
// @Tags logs
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /logs [get]
// @Router /logs/clear [post]
func swaggerDocLogRoutes() {}

// swaggerDocFileScanRoutes documents file scan APIs.
//
// @Summary File scan APIs
// @Description Triggers scan, previews importable files, imports selected files, and cleans completed files.
// @Tags filescan
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /filescan/trigger [post]
// @Router /filescan/preview [get]
// @Router /filescan/import [post]
// @Router /filescan/cleanPreview [get]
// @Router /filescan/cleanSelected [post]
// @Router /filescan/cleanCompleted [post]
func swaggerDocFileScanRoutes() {}

// swaggerDocAgentRoutes documents agent management APIs.
//
// @Summary Agent management APIs
// @Description Detects a configured Agent, checks recording files locally or remotely, and generates the installer command.
// @Tags agent
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /agent/detect [get]
// @Router /agent/files/check [get]
// @Router /agent/install-command [get]
func swaggerDocAgentRoutes() {}

// swaggerDocDataRepairRoutes documents data repair APIs.
//
// @Summary Data repair APIs
// @Description Checks and repairs database consistency.
// @Tags datarepair
// @Security BasicAuth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /datarepair/check [get]
// @Router /datarepair/repair [post]
func swaggerDocDataRepairRoutes() {}

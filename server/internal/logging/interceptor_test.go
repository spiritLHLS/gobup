package logging

import "testing"

func TestInferLogLevelDoesNotTreatZeroFailureSummaryAsError(t *testing.T) {
	message := "[弹幕回补] 扫描完成: 追加尝试=3, 追加失败=0, 限流早停=0"
	if got := inferLogLevel(message); got != "INFO" {
		t.Fatalf("inferLogLevel() = %s, want INFO", got)
	}
}

func TestInferLogLevelDetectsActualFailure(t *testing.T) {
	message := "[弹幕回补] 重新追加失败: burned_part_id=123, err=HTTP 406"
	if got := inferLogLevel(message); got != "ERROR" {
		t.Fatalf("inferLogLevel() = %s, want ERROR", got)
	}
}

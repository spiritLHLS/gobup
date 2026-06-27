package bili

import "testing"

func TestParseSeasonListCreativeResponse(t *testing.T) {
	body := []byte(`{
		"code": 0,
		"message": "0",
		"data": {
			"total": 1,
			"seasons": [
				{
					"season": {"id": 3541247, "title": "Debian", "ep_num": 0},
					"sections": {
						"sections": [
							{"id": 3954033, "title": "正片", "state": 0, "partState": 0, "epCount": 2}
						]
					},
					"part_episodes": [{}, {}]
				}
			]
		}
	}`)

	seasons, total, err := parseSeasonList(body)
	if err != nil {
		t.Fatalf("parseSeasonList returned error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(seasons) != 1 {
		t.Fatalf("len(seasons) = %d, want 1", len(seasons))
	}
	season := seasons[0]
	if season.ID != 3541247 || season.Name != "Debian" || season.SectionID != 3954033 || season.Count != 2 {
		t.Fatalf("season = %+v", season)
	}
}

func TestParseSeasonListPrefersHealthySection(t *testing.T) {
	body := []byte(`{
		"code": 0,
		"data": {
			"seasons": [
				{
					"season": {"id": 1, "title": "合集"},
					"sections": {
						"sections": [
							{"id": 10, "state": -6, "partState": 0, "epCount": 1},
							{"id": 11, "state": 0, "partState": 0, "epCount": 3}
						]
					}
				}
			]
		}
	}`)

	seasons, _, err := parseSeasonList(body)
	if err != nil {
		t.Fatalf("parseSeasonList returned error: %v", err)
	}
	if got := seasons[0].SectionID; got != 11 {
		t.Fatalf("SectionID = %d, want 11", got)
	}
	if got := seasons[0].Count; got != 4 {
		t.Fatalf("Count = %d, want 4", got)
	}
}

func TestParseSeasonListLegacyFallback(t *testing.T) {
	body := []byte(`{
		"code": 0,
		"data": {
			"items": [
				{
					"id": 20,
					"meta": {"name": "旧合集", "total": 7},
					"sections": [{"id": 30, "title": "默认"}]
				}
			]
		}
	}`)

	seasons, _, err := parseSeasonList(body)
	if err != nil {
		t.Fatalf("parseSeasonList returned error: %v", err)
	}
	if len(seasons) != 1 {
		t.Fatalf("len(seasons) = %d, want 1", len(seasons))
	}
	if seasons[0].ID != 20 || seasons[0].Name != "旧合集" || seasons[0].SectionID != 30 || seasons[0].Count != 7 {
		t.Fatalf("season = %+v", seasons[0])
	}
}

func TestAppendCookieValueIfMissing(t *testing.T) {
	cookie := "SESSDATA=abc; buvid3=old"
	got := appendCookieValueIfMissing(cookie, "buvid3", "new")
	if got != cookie {
		t.Fatalf("existing key was replaced: %q", got)
	}

	got = appendCookieValueIfMissing(cookie, "buvid4", "v4")
	want := "SESSDATA=abc; buvid3=old; buvid4=v4"
	if got != want {
		t.Fatalf("cookie = %q, want %q", got, want)
	}
}

package store

import (
	"database/sql"
	"sort"
	"strings"
	"time"
)

type CountItem struct {
	Name  string `json:"name"`
	UID   string `json:"uid,omitempty"`
	Count int    `json:"count"`
	Words int    `json:"words,omitempty"`
	Color string `json:"color,omitempty"`
}

type MonthStat struct {
	Month   string `json:"month"`
	Notes   int    `json:"notes"`
	Words   int    `json:"words"`
	Updated int    `json:"updated"`
}

type DayStat struct {
	Date  string `json:"date"`
	Notes int    `json:"notes"`
	Words int    `json:"words"`
	Edits int    `json:"edits"`
}

type NoteBrief struct {
	UID       string `json:"uid"`
	Title     string `json:"title"`
	Words     int    `json:"words"`
	Notebook  string `json:"notebook"`
	UpdatedAt string `json:"updatedAt"`
	CreatedAt string `json:"createdAt"`
}

type Stats struct {
	VaultName      string      `json:"vaultName"`
	TotalNotes     int         `json:"totalNotes"`
	ActiveNotes    int         `json:"activeNotes"`
	ArchivedNotes  int         `json:"archivedNotes"`
	TrashedNotes   int         `json:"trashedNotes"`
	StarredNotes   int         `json:"starredNotes"`
	PinnedNotes    int         `json:"pinnedNotes"`
	TotalWords     int         `json:"totalWords"`
	AvgWords       int         `json:"avgWords"`
	NotebookCount  int         `json:"notebookCount"`
	TagCount       int         `json:"tagCount"`
	AttachmentNum  int         `json:"attachmentCount"`
	AttachmentSize int64       `json:"attachmentSize"`
	CreatedToday   int         `json:"createdToday"`
	UpdatedToday   int         `json:"updatedToday"`
	CreatedWeek    int         `json:"createdWeek"`
	CreatedMonth   int         `json:"createdMonth"`
	Streak         int         `json:"streak"`
	FirstNoteAt    string      `json:"firstNoteAt"`
	LastNoteAt     string      `json:"lastNoteAt"`
	ByNotebook     []CountItem `json:"byNotebook"`
	ByTag          []CountItem `json:"byTag"`
	ByMonth        []MonthStat `json:"byMonth"`
	ByWeekday      []int       `json:"byWeekday"`
	Daily          []DayStat   `json:"daily"`
	TopNotes       []NoteBrief `json:"topNotes"`
	RecentNotes    []NoteBrief `json:"recentNotes"`
	Types          []CountItem `json:"types"`
	OldestDrafts   int         `json:"oldestDrafts"`
	UnfiledNotes   int         `json:"unfiledNotes"`
	ImportBatches  int         `json:"importBatches"`
	DBBytes        int64       `json:"dbBytes"`
	GeneratedAt    string      `json:"generatedAt"`
	LongestNote    string      `json:"longestNote"`
	BusiestWeekday string      `json:"busiestWeekday"`
}

// Stats computes the numbers behind the dashboard and the statistics export.
func (db *DB) Stats() (*Stats, error) {
	s := &Stats{VaultName: db.GetSetting("vault_name", "我的知识库"), GeneratedAt: Now()}
	s.ByNotebook = []CountItem{}
	s.ByTag = []CountItem{}
	s.ByMonth = []MonthStat{}
	s.ByWeekday = make([]int, 7)
	s.Daily = []DayStat{}
	s.TopNotes = []NoteBrief{}
	s.RecentNotes = []NoteBrief{}
	s.Types = []CountItem{}

	row := db.SQL.QueryRow(`SELECT
		COUNT(*),
		SUM(CASE WHEN deleted_at IS NULL AND status<>'archived' THEN 1 ELSE 0 END),
		SUM(CASE WHEN deleted_at IS NULL AND status='archived' THEN 1 ELSE 0 END),
		SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END),
		SUM(CASE WHEN deleted_at IS NULL AND starred=1 THEN 1 ELSE 0 END),
		SUM(CASE WHEN deleted_at IS NULL AND pinned=1 THEN 1 ELSE 0 END),
		COALESCE(SUM(CASE WHEN deleted_at IS NULL THEN word_count ELSE 0 END),0),
		MIN(CASE WHEN deleted_at IS NULL THEN created_at END),
		MAX(updated_at)
		FROM notes`)
	var first, last sql.NullString
	if err := row.Scan(&s.TotalNotes, &s.ActiveNotes, &s.ArchivedNotes, &s.TrashedNotes, &s.StarredNotes,
		&s.PinnedNotes, &s.TotalWords, &first, &last); err != nil {
		return nil, err
	}
	s.FirstNoteAt = first.String
	s.LastNoteAt = last.String
	if s.ActiveNotes > 0 {
		s.AvgWords = s.TotalWords / s.ActiveNotes
	}

	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM notebooks WHERE deleted_at IS NULL`).Scan(&s.NotebookCount)
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM tags`).Scan(&s.TagCount)
	_ = db.SQL.QueryRow(`SELECT COUNT(*),COALESCE(SUM(size),0) FROM attachments`).Scan(&s.AttachmentNum, &s.AttachmentSize)
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM import_batches`).Scan(&s.ImportBatches)
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM notes WHERE notebook_uid='' OR notebook_uid NOT IN (SELECT uid FROM notebooks WHERE deleted_at IS NULL)`).Scan(&s.UnfiledNotes)
	_ = db.SQL.QueryRow(`SELECT COUNT(*) FROM notes WHERE deleted_at IS NULL AND status<>'archived' AND word_count < 50`).Scan(&s.OldestDrafts)

	// notebook distribution
	rows, err := db.SQL.Query(`SELECT COALESCE(nb.name,'未分类'), n.notebook_uid, COUNT(*), COALESCE(SUM(n.word_count),0), COALESCE(nb.color,'')
		FROM notes n LEFT JOIN notebooks nb ON nb.uid=n.notebook_uid
		WHERE n.deleted_at IS NULL AND n.status<>'archived'
		GROUP BY n.notebook_uid ORDER BY COUNT(*) DESC`)
	if err == nil {
		for rows.Next() {
			var c CountItem
			if rows.Scan(&c.Name, &c.UID, &c.Count, &c.Words, &c.Color) == nil {
				s.ByNotebook = append(s.ByNotebook, c)
			}
		}
		rows.Close()
	}

	tags, _ := db.Tags()
	SortTagsByCount(tags)
	for _, t := range tags {
		if t.Count > 0 {
			s.ByTag = append(s.ByTag, CountItem{Name: t.Name, UID: t.UID, Count: t.Count, Color: t.Color})
		}
	}
	if len(s.ByTag) > 30 {
		s.ByTag = s.ByTag[:30]
	}

	// monthly trend (last 18 months)
	rows2, err := db.SQL.Query(`SELECT substr(created_at,1,7) AS m, COUNT(*), COALESCE(SUM(word_count),0) FROM notes
		WHERE deleted_at IS NULL GROUP BY m ORDER BY m`)
	if err == nil {
		for rows2.Next() {
			var m MonthStat
			if rows2.Scan(&m.Month, &m.Notes, &m.Words) == nil {
				s.ByMonth = append(s.ByMonth, m)
			}
		}
		rows2.Close()
	}
	if len(s.ByMonth) > 18 {
		s.ByMonth = s.ByMonth[len(s.ByMonth)-18:]
	}

	// types
	rows3, err := db.SQL.Query(`SELECT type,COUNT(*) FROM notes WHERE deleted_at IS NULL GROUP BY type ORDER BY COUNT(*) DESC`)
	if err == nil {
		for rows3.Next() {
			var c CountItem
			if rows3.Scan(&c.Name, &c.Count) == nil {
				s.Types = append(s.Types, c)
			}
		}
		rows3.Close()
	}

	// daily activity for the last 365 days, based on last edit date
	start := time.Now().AddDate(0, 0, -364).Format("2006-01-02")
	rows4, err := db.SQL.Query(`SELECT substr(updated_at,1,10) AS d, COUNT(*), COALESCE(SUM(word_count),0)
		FROM notes WHERE deleted_at IS NULL AND updated_at>=? GROUP BY d ORDER BY d`, start)
	dayMap := map[string]*DayStat{}
	if err == nil {
		for rows4.Next() {
			d := DayStat{}
			if rows4.Scan(&d.Date, &d.Edits, &d.Words) == nil {
				dayMap[d.Date] = &d
			}
		}
		rows4.Close()
	}
	// created per day
	rows5, err := db.SQL.Query(`SELECT substr(created_at,1,10) AS d, COUNT(*) FROM notes WHERE deleted_at IS NULL AND created_at>=? GROUP BY d`, start)
	createdMap := map[string]int{}
	if err == nil {
		for rows5.Next() {
			var d string
			var c int
			if rows5.Scan(&d, &c) == nil {
				createdMap[d] = c
			}
		}
		rows5.Close()
	}
	for i := 364; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		d := DayStat{Date: date}
		if v, ok := dayMap[date]; ok {
			d.Edits = v.Edits
			d.Words = v.Words
		}
		d.Notes = createdMap[date]
		s.Daily = append(s.Daily, d)
		if t, err := time.Parse("2006-01-02", date); err == nil {
			s.ByWeekday[int(t.Weekday())] += d.Edits
		}
	}
	// streak: consecutive days with activity, ending today or yesterday
	activeDays := map[string]bool{}
	for _, d := range s.Daily {
		if d.Edits > 0 {
			activeDays[d.Date] = true
		}
	}
	cursor := time.Now()
	if !activeDays[cursor.Format("2006-01-02")] {
		cursor = cursor.AddDate(0, 0, -1)
	}
	for activeDays[cursor.Format("2006-01-02")] {
		s.Streak++
		cursor = cursor.AddDate(0, 0, -1)
	}

	today := time.Now().Format("2006-01-02")
	weekAgo := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	monthStart := time.Now().Format("2006-01") + "-01"
	_ = db.SQL.QueryRow(`SELECT
		SUM(CASE WHEN substr(created_at,1,10)=? THEN 1 ELSE 0 END),
		SUM(CASE WHEN substr(updated_at,1,10)=? THEN 1 ELSE 0 END),
		SUM(CASE WHEN substr(created_at,1,10)>=? THEN 1 ELSE 0 END),
		SUM(CASE WHEN substr(created_at,1,10)>=? THEN 1 ELSE 0 END)
		FROM notes WHERE deleted_at IS NULL`, today, today, weekAgo, monthStart).
		Scan(&s.CreatedToday, &s.UpdatedToday, &s.CreatedWeek, &s.CreatedMonth)

	// top notes by words
	rows6, err := db.SQL.Query(`SELECT n.uid,n.title,n.word_count,COALESCE(nb.name,''),n.updated_at,n.created_at
		FROM notes n LEFT JOIN notebooks nb ON nb.uid=n.notebook_uid
		WHERE n.deleted_at IS NULL AND n.status<>'archived' ORDER BY n.word_count DESC LIMIT 10`)
	if err == nil {
		for rows6.Next() {
			var b NoteBrief
			if rows6.Scan(&b.UID, &b.Title, &b.Words, &b.Notebook, &b.UpdatedAt, &b.CreatedAt) == nil {
				s.TopNotes = append(s.TopNotes, b)
			}
		}
		rows6.Close()
	}
	rows7, err := db.SQL.Query(`SELECT n.uid,n.title,n.word_count,COALESCE(nb.name,''),n.updated_at,n.created_at
		FROM notes n LEFT JOIN notebooks nb ON nb.uid=n.notebook_uid
		WHERE n.deleted_at IS NULL AND n.status<>'archived' ORDER BY n.updated_at DESC LIMIT 10`)
	if err == nil {
		for rows7.Next() {
			var b NoteBrief
			if rows7.Scan(&b.UID, &b.Title, &b.Words, &b.Notebook, &b.UpdatedAt, &b.CreatedAt) == nil {
				s.RecentNotes = append(s.RecentNotes, b)
			}
		}
		rows7.Close()
	}
	if len(s.TopNotes) > 0 {
		s.LongestNote = s.TopNotes[0].Title
	}
	weekdays := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	best := 0
	for i, v := range s.ByWeekday {
		if v > s.ByWeekday[best] {
			best = i
		}
	}
	s.BusiestWeekday = weekdays[best]
	if info, err := db.SQL.Query(`SELECT 1`); err == nil {
		info.Close()
	}
	return s, nil
}

// TagCloud returns tags ordered by usage for pickers.
func (db *DB) TagCloud() []Tag {
	tags, _ := db.Tags()
	SortTagsByCount(tags)
	return tags
}

// MonthlyExportRows prepares the flat rows used by the statistics spreadsheet.
func (db *DB) MonthlyExportRows() []MonthStat {
	rows, err := db.SQL.Query(`SELECT substr(created_at,1,7) AS m, COUNT(*), COALESCE(SUM(word_count),0) FROM notes
		WHERE deleted_at IS NULL GROUP BY m ORDER BY m`)
	if err != nil {
		return []MonthStat{}
	}
	defer rows.Close()
	out := []MonthStat{}
	for rows.Next() {
		var m MonthStat
		if rows.Scan(&m.Month, &m.Notes, &m.Words) == nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out
}

func monthLabel(m string) string {
	parts := strings.Split(m, "-")
	if len(parts) == 2 {
		return parts[0] + "年" + strings.TrimLeft(parts[1], "0") + "月"
	}
	return m
}

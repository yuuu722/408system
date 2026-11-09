package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/local/exam408-assistant/backend/internal/domain"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

func (p *Postgres) Memories(ctx context.Context, uid string) ([]domain.LearnerMemory, error) {
	out := []domain.LearnerMemory{}
	rows, e := p.DB.Query(ctx, `SELECT memory_key,COALESCE(point_id::text,''),kind,content,resolved,updated_at FROM learner_memories WHERE user_id=$1 ORDER BY updated_at DESC LIMIT 100`, uid)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var m domain.LearnerMemory
		if e = rows.Scan(&m.Key, &m.PointID, &m.Kind, &m.Content, &m.Resolved, &m.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (p *Postgres) SaveMemory(ctx context.Context, uid string, m domain.LearnerMemory) error {
	m.Content = strings.TrimSpace(m.Content)
	if m.PointID != "" {
		if _, e := uuid.Parse(m.PointID); e != nil {
			return &domain.RequestError{Status: 400, Message: "知识点标识无效"}
		}
		var exists bool
		if e := p.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM exam408_knowledge_points WHERE id=$1)`, m.PointID).Scan(&exists); e != nil {
			return e
		}
		if !exists {
			return &domain.RequestError{Status: 400, Message: "知识点不存在"}
		}
	}

	if len(m.Key) == 0 || len(m.Key) > 100 || len([]rune(m.Content)) < 1 || len([]rune(m.Content)) > 500 || (m.Kind != "preference" && m.Kind != "goal" && m.Kind != "doubt") {
		return &domain.RequestError{Status: 400, Message: "记忆内容或类型不正确"}
	}
	var count int
	if e := p.DB.QueryRow(ctx, `SELECT count(*) FROM learner_memories WHERE user_id=$1 AND memory_key<>$2`, uid, m.Key).Scan(&count); e != nil {
		return e
	}
	if count >= 100 {
		return &domain.RequestError{Status: 400, Message: "最多保存100条记忆，请先整理旧记录"}
	}
	_, e := p.DB.Exec(ctx, `INSERT INTO learner_memories(user_id,memory_key,point_id,kind,content,resolved) VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6) ON CONFLICT(user_id,memory_key) DO UPDATE SET point_id=EXCLUDED.point_id,kind=EXCLUDED.kind,content=EXCLUDED.content,resolved=EXCLUDED.resolved,updated_at=now()`, uid, m.Key, m.PointID, m.Kind, m.Content, m.Resolved)
	return e
}
func (p *Postgres) DeleteMemory(ctx context.Context, uid, key string) error {
	_, e := p.DB.Exec(ctx, `DELETE FROM learner_memories WHERE user_id=$1 AND memory_key=$2`, uid, key)
	return e
}
func (p *Postgres) RecordQuestionDoubts(ctx context.Context, uid, q string) error {
	if !strings.Contains(q, "不懂") && !strings.Contains(q, "不会") && !strings.Contains(q, "不明白") && !strings.Contains(q, "为什么") && !strings.Contains(q, "没理解") && !strings.Contains(q, "如何") && !strings.Contains(q, "怎么") && !strings.Contains(q, "是什么") && !strings.Contains(q, "比较") {
		return nil
	}
	points, e := p.ListKnowledgePoints(ctx)
	if e != nil {
		return e
	}
	n := 0
	for _, k := range points {
		match := strings.Contains(q, k.Name)
		for _, word := range k.Keywords {
			if len([]rune(word)) >= 3 && strings.Contains(q, word) {
				match = true
			}
		}
		if match {
			_, e = p.DB.Exec(ctx, `INSERT INTO learner_memories(user_id,memory_key,point_id,kind,content) SELECT $1,$2,$3,'doubt',$4 WHERE (SELECT count(*) FROM learner_memories WHERE user_id=$1)<100 ON CONFLICT(user_id,memory_key) DO NOTHING`, uid, "question:"+k.ID, k.ID, "在问答中对“"+k.Name+"”提出疑问；待练习验证，不代表答错。")
			if e != nil {
				return e
			}
			n++
			if n >= 3 {
				break
			}
		}
	}
	return nil
}
func (p *Postgres) FrequencySources(ctx context.Context) ([]domain.FrequencySource, error) {
	out := []domain.FrequencySource{}
	rows, e := p.DB.Query(ctx, `SELECT id,point_id,organization,title,url,excerpt,start_year,end_year,exams,appearances FROM point_frequency_sources ORDER BY end_year DESC,organization,point_id`)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v domain.FrequencySource
		if e = rows.Scan(&v.ID, &v.PointID, &v.Organization, &v.Title, &v.URL, &v.Excerpt, &v.StartYear, &v.EndYear, &v.Exams, &v.Appearances); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (p *Postgres) SaveFrequencySource(ctx context.Context, uid string, v domain.FrequencySource) error {
	u, e := p.UserByID(ctx, uid)
	if e != nil {
		return e
	}
	if u.Role != "admin" {
		return domain.ErrForbidden
	}
	v.Organization = strings.TrimSpace(v.Organization)
	if _, e := uuid.Parse(v.PointID); e != nil {
		return &domain.RequestError{Status: 400, Message: "知识点标识无效"}
	}
	link, e := url.Parse(v.URL)
	if e != nil || link.Scheme != "https" || link.Hostname() == "" || link.User != nil || len(v.URL) > 2000 || strings.TrimSpace(v.Organization) == "" || len([]rune(v.Organization)) > 100 || len([]rune(v.Title)) < 3 || len([]rune(v.Title)) > 300 || len([]rune(v.Excerpt)) < 10 || len([]rune(v.Excerpt)) > 3000 || v.StartYear < 2009 || v.EndYear > 2100 || v.StartYear > v.EndYear || v.Exams < 1 || v.Exams > v.EndYear-v.StartYear+1 || v.Appearances < 0 || v.Appearances > v.Exams {
		return &domain.RequestError{Status: 400, Message: "考频必须提供机构、HTTPS原文链接、摘录、年份区间及按年度出现的分子分母；不接受无来源百分比"}
	}
	_, e = p.DB.Exec(ctx, `INSERT INTO point_frequency_sources(point_id,organization,title,url,excerpt,start_year,end_year,exams,appearances) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(point_id,organization,start_year,end_year) DO UPDATE SET title=EXCLUDED.title,url=EXCLUDED.url,excerpt=EXCLUDED.excerpt,exams=EXCLUDED.exams,appearances=EXCLUDED.appearances,updated_at=now()`, v.PointID, v.Organization, v.Title, v.URL, v.Excerpt, v.StartYear, v.EndYear, v.Exams, v.Appearances)
	return e
}

var examYearPattern = regexp.MustCompile(`20[0-3][0-9]`)

func (p *Postgres) LearningOverview(ctx context.Context, uid string) (domain.LearningOverview, error) {
	out := domain.LearningOverview{Points: []domain.PointInsight{}, NeedsPretest: true, Notice: "前测仅覆盖当前有可靠题目的知识点；少量题目不能确定全面能力。疑问记录与有效作答分开。考频按年度出现统计，不等于未来出题概率；不同年份口径不直接平均。"}
	points, e := p.ListKnowledgePoints(ctx)
	if e != nil {
		return out, e
	}
	out.Memories, e = p.Memories(ctx, uid)
	if e != nil {
		return out, e
	}
	sources, e := p.FrequencySources(ctx)
	if e != nil {
		return out, e
	}
	tested, correct := map[string]int{}, map[string]int{}
	rows, e := p.DB.Query(ctx, `WITH latest AS (SELECT DISTINCT ON (`+canonicalEventFamilySQL+`) knowledge_point_id,event_type FROM exam408_learning_events WHERE user_id=$1 AND event_type IN ('quiz_correct','quiz_wrong') AND metadata ? 'question_id' AND NOT EXISTS(SELECT 1 FROM learning_assets invalid WHERE invalid.id::text=metadata->>'question_id' AND invalid.payload->>'ReviewStatus'='source_conflict') ORDER BY `+canonicalEventFamilySQL+`,created_at DESC,metadata->>'assessment_id' DESC,metadata->>'question_id' DESC) SELECT knowledge_point_id,count(*),count(*) FILTER(WHERE event_type='quiz_correct') FROM latest GROUP BY knowledge_point_id`, uid)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var id string
		var n, c int
		if e = rows.Scan(&id, &n, &c); e != nil {
			rows.Close()
			return out, e
		}
		tested[id] = n
		correct[id] = c
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	assets, e := p.Assets(ctx, uid)
	if e != nil {
		return out, e
	}
	available := map[string]int{}
	seen := map[string]bool{}
	for _, a := range assets {
		if a.Kind == "question" && a.Status == "ready" && a.Payload.AnswerOrigin == "source" && !seen[domain.Family(a)] {
			available[a.PointID]++
			seen[domain.Family(a)] = true
		}
	}
	subtreeCounts := domain.SubtreeQuestionCounts(points, available)
	years := map[string]map[string]bool{}
	corpus := map[string]map[string]bool{}
	pointSubjects := map[string]string{}
	for _, pt := range points {
		pointSubjects[pt.ID] = pt.SubjectID
	}
	rows, e = p.DB.Query(ctx, `SELECT a.point_id::text,d.original_filename,COALESCE(a.payload->>'SectionKey','') FROM learning_assets a JOIN documents d ON d.id=a.document_id WHERE a.is_public AND a.kind='question' AND a.status='ready' AND a.payload->>'AnswerOrigin'='source' AND d.resource_type='past_exam'`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var id, name, section string
		if e = rows.Scan(&id, &name, &section); e != nil {
			rows.Close()
			return out, e
		}
		if strings.Contains(name, "模拟") || strings.Contains(name, "风格") {
			continue
		}
		ys := examYearPattern.FindAllString(name, -1)
		year := ""
		if len(ys) == 1 {
			year = ys[0]
		} else if examYearPattern.MatchString(section) {
			year = examYearPattern.FindString(section)
		}
		if year == "" {
			continue
		}
		if years[id] == nil {
			years[id] = map[string]bool{}
		}
		years[id][year] = true
		subject := pointSubjects[id]
		if corpus[subject] == nil {
			corpus[subject] = map[string]bool{}
		}
		corpus[subject][year] = true
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, k := range points {
		v := domain.PointInsight{ID: k.ID, Code: k.Code, Name: k.Name, ParentID: k.ParentID, SubjectID: k.SubjectID, SubjectName: k.SubjectName, Description: k.Description, Tested: tested[k.ID], Correct: correct[k.ID], AvailableQuestions: available[k.ID], SubtreeQuestions: subtreeCounts[k.ID], ExamYears: len(years[k.ID]), CorpusYears: len(corpus[k.SubjectID]), State: "待测", Importance: 1, ImportanceBasis: "目录基础顺序；尚无可靠考频", Sources: []domain.FrequencySource{}}
		if v.Tested > 0 {
			ratio := float64(v.Correct) / float64(v.Tested)
			v.Accuracy = &ratio
			v.State = "样本不足"
			if v.Tested < 3 && v.Correct < v.Tested {
				v.State = "待复查（有错题）"
			}
			if v.Tested >= 3 {
				if ratio < 0.6 {
					v.State = "需要补强"
				} else if ratio >= 0.85 {
					v.State = "目前表现较稳"
				} else {
					v.State = "继续巩固"
				}
			}
		}
		for _, m := range out.Memories {
			if m.PointID == k.ID && m.Kind == "doubt" && !m.Resolved {
				v.Doubts++
			}
		}
		if v.CorpusYears > 0 {
			v.Importance = 1 + 4*float64(v.ExamYears)/float64(v.CorpusYears)
			v.ImportanceBasis = "库内已核对真题年度覆盖（不是全量考频）"
		}
		compatible := map[string][]domain.FrequencySource{}
		for _, f := range sources {
			if f.PointID == k.ID {
				v.Sources = append(v.Sources, f)
				key := fmt.Sprintf("%d-%d/%d", f.StartYear, f.EndYear, f.Exams)
				compatible[key] = append(compatible[key], f)
			}
		}
		latest, selectedExams := 0, 0
		keys := []string{}
		for key := range compatible {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fs := compatible[key]
			if len(fs) >= 2 && (fs[0].EndYear > latest || (fs[0].EndYear == latest && fs[0].Exams > selectedExams)) {
				sum := 0.0
				for _, f := range fs {
					sum += float64(f.Appearances) / float64(f.Exams)
				}
				v.Importance = 1 + 4*sum/float64(len(fs))
				v.ImportanceBasis = fmt.Sprintf("%d家机构，%d–%d年/%d年度，同口径占比均值（需核对原文）", len(fs), fs[0].StartYear, fs[0].EndYear, fs[0].Exams)
				latest = fs[0].EndYear
				selectedExams = fs[0].Exams
			}
		}
		weak := 0.5
		if v.Accuracy != nil {
			weak = 1 - *v.Accuracy
		}
		v.Priority = 40*weak + 8*v.Importance + float64(v.Doubts)*8
		if v.Tested == 0 {
			v.Priority += 5
		}
		out.Points = append(out.Points, v)
	}
	var n int
	e = p.DB.QueryRow(ctx, `SELECT count(*) FROM assessments WHERE user_id=$1 AND kind='diagnostic' AND result IS NOT NULL`, uid).Scan(&n)
	out.NeedsPretest = n == 0
	sort.SliceStable(out.Points, func(i, j int) bool {
		if out.Points[i].Priority == out.Points[j].Priority {
			return out.Points[i].Code < out.Points[j].Code
		}
		return out.Points[i].Priority > out.Points[j].Priority
	})
	return out, e
}
func (p *Postgres) SaveCoachSession(ctx context.Context, uid, kid, point, teacher string, a domain.Assessment, lesson domain.ChatResult) (domain.CoachSession, error) {
	v := domain.CoachSession{PointID: point, Teacher: teacher, KnowledgeBaseID: kid, Assessment: a, Lesson: lesson}
	raw, e := json.Marshal(lesson)
	if e != nil {
		return v, e
	}
	e = p.DB.QueryRow(ctx, `INSERT INTO coaching_sessions(user_id,assessment_id,point_id,teacher,lesson,knowledge_base_id) SELECT $1,a.id,$3,$4,$5,$6 FROM assessments a WHERE a.id=$2 AND a.user_id=$1 RETURNING id,created_at`, uid, a.ID, point, teacher, raw, kid).Scan(&v.ID, &v.CreatedAt)
	return v, e
}
func (p *Postgres) CoachSessions(ctx context.Context, uid string) ([]domain.CoachSession, error) {
	out := []domain.CoachSession{}
	rows, e := p.DB.Query(ctx, `SELECT id,point_id,teacher,lesson,feedback,assessment_id,created_at,knowledge_base_id FROM coaching_sessions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 10`, uid)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v domain.CoachSession
		var raw, feedback []byte
		var aid string
		if e = rows.Scan(&v.ID, &v.PointID, &v.Teacher, &raw, &feedback, &aid, &v.CreatedAt, &v.KnowledgeBaseID); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal(raw, &v.Lesson); e != nil {
			rows.Close()
			return nil, e
		}
		if len(feedback) > 0 {
			if e = json.Unmarshal(feedback, &v.Feedback); e != nil {
				rows.Close()
				return nil, e
			}
		}
		v.Assessment.ID = aid
		out = append(out, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for i := range out {
		a, e := p.Assessment(ctx, uid, out[i].Assessment.ID)
		if e != nil {
			return nil, e
		}
		out[i].Assessment = RedactAssessment(a)
	}
	return out, nil
}

func (p *Postgres) SaveCoachFeedback(ctx context.Context, uid, id string, v domain.ChatResult) error {
	raw, e := json.Marshal(v)
	if e != nil {
		return e
	}
	tag, e := p.DB.Exec(ctx, `UPDATE coaching_sessions SET feedback=$3 WHERE id=$1 AND user_id=$2`, id, uid, raw)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return e
}

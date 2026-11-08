package v2

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func initDesignPlans(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS design_plans (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS design_questions (id INTEGER PRIMARY KEY AUTOINCREMENT, plan_id INTEGER NOT NULL REFERENCES design_plans(id), title TEXT NOT NULL, sort_order INTEGER NOT NULL, UNIQUE(plan_id, sort_order))`,
		`CREATE TABLE IF NOT EXISTS design_plan_classes (plan_id INTEGER NOT NULL REFERENCES design_plans(id), class_id INTEGER NOT NULL REFERENCES classes(id), PRIMARY KEY(plan_id,class_id))`,
		`CREATE TABLE IF NOT EXISTS design_versions (plan_id INTEGER NOT NULL REFERENCES design_plans(id), student_id INTEGER NOT NULL REFERENCES students(id), class_id INTEGER NOT NULL REFERENCES classes(id), version INTEGER NOT NULL CHECK(version IN (1,2)), title TEXT NOT NULL, questions_json TEXT NOT NULL, answers_json TEXT NOT NULL, submitted_at TEXT NOT NULL, PRIMARY KEY(plan_id,student_id,version))`,
		`CREATE INDEX IF NOT EXISTS idx_design_versions_class ON design_versions(plan_id,class_id)`,
		`CREATE TABLE IF NOT EXISTS design_class_progress (plan_id INTEGER NOT NULL REFERENCES design_plans(id), class_id INTEGER NOT NULL REFERENCES classes(id), stage INTEGER NOT NULL DEFAULT 1 CHECK(stage BETWEEN 1 AND 3), updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(plan_id,class_id))`,
		`INSERT OR IGNORE INTO design_class_progress(plan_id,class_id) SELECT plan_id,class_id FROM design_plan_classes`,
		`CREATE TABLE IF NOT EXISTS design_class_active_plan(class_id INTEGER PRIMARY KEY REFERENCES classes(id),plan_id INTEGER NOT NULL REFERENCES design_plans(id))`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := ensureColumn(db, "design_plans", "output_type", "TEXT NOT NULL DEFAULT 'image' CHECK(output_type IN ('image','html'))"); err != nil {
		return err
	}
	if err := initDesignImages(db); err != nil {
		return err
	}
	return initDesignHTML(db)
}

type DesignPlan struct {
	OutputType       string   `json:"outputType"`
	OutputTypeLocked bool     `json:"outputTypeLocked"`
	ID               int      `json:"id"`
	Title            string   `json:"title"`
	Questions        []string `json:"questions"`
	ClassIDs         []int    `json:"classIds"`
	Locked           bool     `json:"locked"`
}

type DesignVersion struct {
	Version     int      `json:"version"`
	Title       string   `json:"title"`
	Questions   []string `json:"questions"`
	Answers     []string `json:"answers"`
	SubmittedAt string   `json:"submittedAt"`
}

type DesignStudentRecord struct {
	HTMLVersionCount   int             `json:"htmlVersionCount"`
	FinalHTMLVersionID string          `json:"finalHtmlVersionId"`
	HTMLFinalizedAt    string          `json:"htmlFinalizedAt"`
	StudentID          int             `json:"studentId"`
	ClassID            int             `json:"classId"`
	Name               string          `json:"name"`
	ClassName          string          `json:"className"`
	Versions           []DesignVersion `json:"versions"`
	Stage              int             `json:"stage"`
	ImageCount         int             `json:"imageCount"`
	FinalImageID       string          `json:"finalImageId"`
	FinalizedAt        string          `json:"finalizedAt"`
}

type designQuery interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

func loadDesignPlan(q designQuery, id int) (*DesignPlan, error) {
	p := &DesignPlan{ID: id, Questions: []string{}, ClassIDs: []int{}}
	if err := q.QueryRow(`SELECT title,output_type, EXISTS(SELECT 1 FROM design_versions WHERE plan_id=?),EXISTS(SELECT 1 FROM design_image_jobs j JOIN design_image_sessions s ON s.id=j.session_id WHERE s.plan_id=?) OR EXISTS(SELECT 1 FROM design_html_jobs j JOIN design_html_sessions s ON s.id=j.session_id WHERE s.plan_id=?) FROM design_plans WHERE id=?`, id, id, id, id).Scan(&p.Title, &p.OutputType, &p.Locked, &p.OutputTypeLocked); err != nil {
		return nil, err
	}
	rows, err := q.Query(`SELECT title FROM design_questions WHERE plan_id=? ORDER BY sort_order`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var title string
		if err = rows.Scan(&title); err != nil {
			rows.Close()
			return nil, err
		}
		p.Questions = append(p.Questions, title)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(`SELECT class_id FROM design_plan_classes WHERE plan_id=? ORDER BY class_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var classID int
		if err = rows.Scan(&classID); err != nil {
			return nil, err
		}
		p.ClassIDs = append(p.ClassIDs, classID)
	}
	return p, rows.Err()
}

func (r *Repository) ListDesignPlans(classID int) ([]DesignPlan, error) {
	rows, err := r.db.Query(`SELECT id FROM design_plans WHERE (?=0 OR EXISTS(SELECT 1 FROM design_plan_classes WHERE plan_id=design_plans.id AND class_id=?)) ORDER BY id DESC`, classID, classID)
	if err != nil {
		return nil, err
	}
	ids := []int{}
	for rows.Next() {
		var id int
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	plans := []DesignPlan{}
	for _, id := range ids {
		p, err := loadDesignPlan(r.db, id)
		if err != nil {
			return nil, err
		}
		plans = append(plans, *p)
	}
	return plans, nil
}

func (r *Repository) SaveDesignPlan(p DesignPlan) (*DesignPlan, error) {
	if len(p.ClassIDs) > 200 {
		return nil, fmt.Errorf("一次最多绑定200个班级")
	}
	var err error
	p.Title, err = normalizeDesignText(p.Title, "方案标题", designTitleLimit, true)
	if err != nil {
		return nil, err
	}
	if p.Title == "" || len(p.Questions) == 0 {
		return nil, fmt.Errorf("请填写方案标题并至少添加一道问题")
	}
	if len(p.Questions) > designQuestionCount {
		return nil, fmt.Errorf("最多%d道问题", designQuestionCount)
	}
	total := 0
	for i := range p.Questions {
		p.Questions[i], err = normalizeDesignText(p.Questions[i], fmt.Sprintf("第%d道问题", i+1), designQuestionLimit, true)
		if err != nil {
			return nil, err
		}
		total += utf8.RuneCountInString(p.Questions[i])
	}
	if total > designQuestionsTotal {
		return nil, fmt.Errorf("问题合计最多%d字", designQuestionsTotal)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if p.ID == 0 {
		if p.OutputType == "" {
			p.OutputType = "image"
		}
		res, err := tx.Exec(`INSERT INTO design_plans(title) VALUES(?)`, p.Title)
		if err != nil {
			return nil, err
		}
		id, _ := res.LastInsertId()
		p.ID = int(id)
	} else {
		// Acquire the write lock before checking submissions, serializing edits and submissions.
		res, err := tx.Exec(`UPDATE design_plans SET title=title WHERE id=?`, p.ID)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return nil, sql.ErrNoRows
		}
		old, err := loadDesignPlan(tx, p.ID)
		if err != nil {
			return nil, err
		}
		if p.OutputType == "" {
			p.OutputType = old.OutputType
		}
		if old.OutputTypeLocked && p.OutputType != old.OutputType {
			return nil, fmt.Errorf("已发起作品生成，作品类型已锁定，请新建方案")
		}
		if old.Locked {
			a, _ := json.Marshal(old.Questions)
			b, _ := json.Marshal(p.Questions)
			if string(a) != string(b) {
				return nil, fmt.Errorf("已有学生提交第一版，题目及顺序已锁定，请另建方案")
			}
		}
	}
	if p.OutputType != "image" && p.OutputType != "html" {
		return nil, fmt.Errorf("不支持的作品类型")
	}
	if _, err = tx.Exec(`UPDATE design_plans SET title=?,output_type=? WHERE id=?`, p.Title, p.OutputType, p.ID); err != nil {
		return nil, err
	}
	var submitted bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM design_versions WHERE plan_id=?)`, p.ID).Scan(&submitted); err != nil {
		return nil, err
	}
	if !submitted {
		if _, err = tx.Exec(`DELETE FROM design_questions WHERE plan_id=?`, p.ID); err != nil {
			return nil, err
		}
		for i, title := range p.Questions {
			if _, err = tx.Exec(`INSERT INTO design_questions(plan_id,title,sort_order) VALUES(?,?,?)`, p.ID, title, i); err != nil {
				return nil, err
			}
		}
	}
	if _, err = tx.Exec(`DELETE FROM design_plan_classes WHERE plan_id=?`, p.ID); err != nil {
		return nil, err
	}
	for _, id := range p.ClassIDs {
		var exists bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM classes WHERE id=? AND deleted_at IS NULL)`, id).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("绑定班级不存在")
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO design_plan_classes(plan_id,class_id) VALUES(?,?)`, p.ID, id); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO design_class_progress(plan_id,class_id) VALUES(?,?)`, p.ID, id); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return loadDesignPlan(r.db, p.ID)
}

func validateDesignStudent(q designQuery, planID, classID, studentID int) error {
	var ok bool
	err := q.QueryRow(`SELECT EXISTS(SELECT 1 FROM students s JOIN classes c ON c.id=s.class_id JOIN design_plan_classes b ON b.class_id=c.id WHERE s.id=? AND c.id=? AND b.plan_id=? AND s.deleted_at IS NULL AND c.deleted_at IS NULL)`, studentID, classID, planID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("学生不属于该班级，或该班级未绑定此方案")
	}
	var active bool
	if err = q.QueryRow(`SELECT EXISTS(SELECT 1 FROM design_class_active_plan WHERE class_id=? AND plan_id=?)`, classID, planID).Scan(&active); err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("老师已切换当前方案，请重新进入")
	}
	return nil
}

func (r *Repository) SetActiveDesignPlan(classID, planID int) error {
	_, err := r.DesignProgress(planID, classID)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(`INSERT INTO design_class_active_plan(class_id,plan_id) VALUES(?,?) ON CONFLICT(class_id) DO UPDATE SET plan_id=excluded.plan_id`, classID, planID)
	return err
}
func (r *Repository) ActiveDesignPlan(classID int) (*DesignPlan, error) {
	var id int
	err := r.db.QueryRow(`SELECT a.plan_id FROM design_class_active_plan a JOIN design_plan_classes b ON b.class_id=a.class_id AND b.plan_id=a.plan_id JOIN classes c ON c.id=a.class_id WHERE a.class_id=? AND c.deleted_at IS NULL`, classID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return loadDesignPlan(r.db, id)
}

func loadDesignVersions(q designQuery, planID, studentID int) ([]DesignVersion, error) {
	rows, err := q.Query(`SELECT version,title,questions_json,answers_json,submitted_at FROM design_versions WHERE plan_id=? AND student_id=? ORDER BY version`, planID, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := []DesignVersion{}
	for rows.Next() {
		var v DesignVersion
		var qs, ans string
		if err = rows.Scan(&v.Version, &v.Title, &qs, &ans, &v.SubmittedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(qs), &v.Questions); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(ans), &v.Answers); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func designComparable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.Is(unicode.Cf, r) || (r >= 0xfe00 && r <= 0xfe0f) || (r >= 0xe0100 && r <= 0xe01ef) {
			return -1
		}
		return r
	}, s)
}

func (r *Repository) SubmitDesignVersion(planID, classID, studentID, version int, answers []string) error {
	if version != 1 && version != 2 {
		return fmt.Errorf("只能提交第一版或第二版")
	}
	answers, err := normalizeDesignAnswers(answers)
	if err != nil {
		return err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE design_plans SET title=title WHERE id=?`, planID); err != nil {
		return err
	}
	if err = validateDesignStudent(tx, planID, classID, studentID); err != nil {
		return err
	}
	p, err := loadDesignPlan(tx, planID)
	if err != nil {
		return err
	}
	versions, err := loadDesignVersions(tx, planID, studentID)
	if err != nil {
		return err
	}
	if version != len(versions)+1 {
		return fmt.Errorf("版本已提交或提交顺序有误，请重新读取方案")
	}
	if version == 2 {
		stage, err := designStage(tx, planID, classID)
		if err != nil {
			return err
		}
		if stage < 2 {
			return fmt.Errorf("等待老师开始第二版")
		}
	}
	questions := p.Questions
	title := p.Title
	if version == 2 {
		questions = versions[0].Questions
		title = versions[0].Title
	}
	if len(questions) == 0 || len(answers) != len(questions) {
		return fmt.Errorf("请完成全部问题")
	}
	changed := false
	for i := range answers {
		answers[i] = strings.TrimSpace(answers[i])
		if answers[i] == "" {
			return fmt.Errorf("第%d题答案不能为空", i+1)
		}
		if version == 2 && designComparable(answers[i]) != designComparable(versions[0].Answers[i]) {
			changed = true
		}
	}
	if version == 2 && !changed {
		return fmt.Errorf("请至少优化修改一道题的答案，仅调整空白不算修改")
	}
	qs, _ := json.Marshal(questions)
	ans, _ := json.Marshal(answers)
	_, err = tx.Exec(`INSERT INTO design_versions(plan_id,class_id,student_id,version,title,questions_json,answers_json,submitted_at) VALUES(?,?,?,?,?,?,?,?)`, planID, classID, studentID, version, title, string(qs), string(ans), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) DesignRecords(planID, classID int) ([]DesignStudentRecord, error) {
	if _, err := loadDesignPlan(r.db, planID); err != nil {
		return nil, err
	}
	// Include historical submitters even if their class is no longer bound or roster entry is deleted.
	rows, err := r.db.Query(`SELECT s.id,c.id,s.name,c.name FROM students s JOIN classes c ON c.id=s.class_id WHERE (?=0 OR c.id=?) AND ((s.deleted_at IS NULL AND c.deleted_at IS NULL AND EXISTS(SELECT 1 FROM design_plan_classes WHERE plan_id=? AND class_id=c.id)) OR EXISTS(SELECT 1 FROM design_versions WHERE plan_id=? AND student_id=s.id)) ORDER BY c.name,s.name,s.id`, classID, classID, planID, planID)
	if err != nil {
		return nil, err
	}
	items := []DesignStudentRecord{}
	for rows.Next() {
		var item DesignStudentRecord
		if err = rows.Scan(&item.StudentID, &item.ClassID, &item.Name, &item.ClassName); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Versions, err = loadDesignVersions(r.db, planID, items[i].StudentID)
		if err != nil {
			return nil, err
		}
		items[i].Stage, err = designStage(r.db, planID, items[i].ClassID)
		if err != nil {
			return nil, err
		}
		err = r.db.QueryRow(`SELECT COALESCE((SELECT COUNT(*) FROM design_images i JOIN design_image_sessions s ON s.id=i.session_id WHERE s.plan_id=? AND s.student_id=?),0),COALESCE((SELECT final_image_id FROM design_image_sessions WHERE plan_id=? AND student_id=?),''),COALESCE((SELECT finalized_at FROM design_image_sessions WHERE plan_id=? AND student_id=?),'')`, planID, items[i].StudentID, planID, items[i].StudentID, planID, items[i].StudentID).Scan(&items[i].ImageCount, &items[i].FinalImageID, &items[i].FinalizedAt)
		if err != nil {
			return nil, err
		}
		if err = r.db.QueryRow(`SELECT COALESCE((SELECT COUNT(*) FROM design_html_versions v JOIN design_html_sessions s ON s.id=v.session_id WHERE s.plan_id=? AND s.student_id=?),0),COALESCE((SELECT final_version_id FROM design_html_sessions WHERE plan_id=? AND student_id=?),''),COALESCE((SELECT finalized_at FROM design_html_sessions WHERE plan_id=? AND student_id=?),'')`, planID, items[i].StudentID, planID, items[i].StudentID, planID, items[i].StudentID).Scan(&items[i].HTMLVersionCount, &items[i].FinalHTMLVersionID, &items[i].HTMLFinalizedAt); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func requireDesignOutputType(q designQuery, planID int, expected string) error {
	var mode string
	if err := q.QueryRow(`SELECT output_type FROM design_plans WHERE id=?`, planID).Scan(&mode); err != nil {
		return err
	}
	if mode != expected {
		return fmt.Errorf("方案作品类型已改变，请刷新后重新确认生成内容")
	}
	return nil
}

func designStage(q designQuery, planID, classID int) (int, error) {
	var stage int
	err := q.QueryRow(`SELECT COALESCE((SELECT stage FROM design_class_progress WHERE plan_id=? AND class_id=?),1)`, planID, classID).Scan(&stage)
	return stage, err
}

func (r *Repository) DesignProgress(planID, classID int) (int, error) {
	var ok bool
	if err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM design_plan_classes b JOIN classes c ON c.id=b.class_id WHERE b.plan_id=? AND b.class_id=? AND c.deleted_at IS NULL)`, planID, classID).Scan(&ok); err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("该班级未绑定此方案")
	}
	return designStage(r.db, planID, classID)
}

func (r *Repository) AdvanceDesignProgress(planID, classID, next int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT OR IGNORE INTO design_class_progress(plan_id,class_id) SELECT plan_id,class_id FROM design_plan_classes WHERE plan_id=? AND class_id=?`, planID, classID); err != nil {
		return err
	}
	var bound bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM design_plan_classes b JOIN classes c ON c.id=b.class_id WHERE b.plan_id=? AND b.class_id=? AND c.deleted_at IS NULL)`, planID, classID).Scan(&bound); err != nil {
		return err
	}
	if !bound {
		return fmt.Errorf("该班级未绑定此方案")
	}
	stage, err := designStage(tx, planID, classID)
	if err != nil {
		return err
	}
	if next != stage+1 || next > 3 {
		return fmt.Errorf("只能依次向后开放下一阶段")
	}
	if _, err = tx.Exec(`UPDATE design_class_progress SET stage=?,updated_at=CURRENT_TIMESTAMP WHERE plan_id=? AND class_id=?`, next, planID, classID); err != nil {
		return err
	}
	return tx.Commit()
}

package v2

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func initDesignHTML(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS design_html_sessions(id INTEGER PRIMARY KEY AUTOINCREMENT,plan_id INTEGER NOT NULL REFERENCES design_plans(id),class_id INTEGER NOT NULL REFERENCES classes(id),student_id INTEGER NOT NULL REFERENCES students(id),snapshot_json TEXT NOT NULL,final_version_id TEXT NOT NULL DEFAULT '',finalized_at TEXT NOT NULL DEFAULT '',UNIQUE(plan_id,student_id))`,
		`CREATE TABLE IF NOT EXISTS design_html_jobs(id TEXT PRIMARY KEY,session_id INTEGER NOT NULL REFERENCES design_html_sessions(id),request_key TEXT NOT NULL,message TEXT NOT NULL,prompt TEXT NOT NULL,source_version_id TEXT NOT NULL DEFAULT '',status TEXT NOT NULL CHECK(status IN ('queued','running','succeeded','failed','interrupted')),error TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(session_id,request_key))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_design_html_active_job ON design_html_jobs(session_id) WHERE status IN ('queued','running')`,
		`CREATE INDEX IF NOT EXISTS idx_design_html_job_queue ON design_html_jobs(status,created_at)`,
		`CREATE TABLE IF NOT EXISTS design_html_versions(id TEXT PRIMARY KEY,session_id INTEGER NOT NULL REFERENCES design_html_sessions(id),job_id TEXT NOT NULL UNIQUE REFERENCES design_html_jobs(id),version INTEGER NOT NULL,parent_version_id TEXT NOT NULL DEFAULT '',file_name TEXT NOT NULL UNIQUE,size_bytes INTEGER NOT NULL,model TEXT NOT NULL,created_at TEXT NOT NULL,UNIQUE(session_id,version))`,
		`CREATE TABLE IF NOT EXISTS design_html_messages(id INTEGER PRIMARY KEY AUTOINCREMENT,session_id INTEGER NOT NULL REFERENCES design_html_sessions(id),job_id TEXT NOT NULL REFERENCES design_html_jobs(id),role TEXT NOT NULL CHECK(role IN ('user','assistant')),content TEXT NOT NULL,version_id TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_design_html_message_session ON design_html_messages(session_id,id)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	for _, name := range []string{"reasoning", "generation_phase", "generation_model"} {
		if err := ensureColumn(db, "design_html_jobs", name, "TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	return nil
}

type DesignHTMLJob struct {
	ID              string `json:"id"`
	SessionID       int    `json:"-"`
	RequestKey      string `json:"requestKey"`
	Message         string `json:"message"`
	Prompt          string `json:"-"`
	SourceVersionID string `json:"sourceVersionId"`
	Status          string `json:"status"`
	Error           string `json:"error"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	Reasoning       string `json:"reasoning,omitempty"`
	GenerationPhase string `json:"generationPhase,omitempty"`
	GenerationModel string `json:"generationModel,omitempty"`
}
type DesignHTMLVersion struct {
	ID              string `json:"id"`
	SessionID       int    `json:"-"`
	JobID           string `json:"jobId"`
	Version         int    `json:"version"`
	ParentVersionID string `json:"parentVersionId"`
	FileName        string `json:"-"`
	SizeBytes       int    `json:"sizeBytes"`
	Model           string `json:"model"`
	CreatedAt       string `json:"createdAt"`
}
type DesignHTMLMessage struct {
	ID        int    `json:"id"`
	JobID     string `json:"jobId"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	VersionID string `json:"versionId"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}
type DesignHTMLState struct {
	SourcePlan      *DesignVersion      `json:"sourcePlan,omitempty"`
	SessionID       int                 `json:"sessionId"`
	FinalVersionID  string              `json:"finalVersionId"`
	FinalizedAt     string              `json:"finalizedAt"`
	VersionCount    int                 `json:"versionCount"`
	Versions        []DesignHTMLVersion `json:"versions"`
	Messages        []DesignHTMLMessage `json:"messages"`
	VersionsHasMore bool                `json:"versionsHasMore"`
	MessagesHasMore bool                `json:"messagesHasMore"`
	LatestVersion   *DesignHTMLVersion  `json:"latestVersion"`
	FinalVersion    *DesignHTMLVersion  `json:"finalVersion"`
	ActiveJob       *DesignHTMLJob      `json:"activeJob"`
	LastJob         *DesignHTMLJob      `json:"lastJob"`
}

const designHTMLJobColumns = `id,session_id,request_key,message,prompt,source_version_id,status,error,created_at,updated_at,reasoning,generation_phase,generation_model`

func scanDesignHTMLJob(row *sql.Row) (*DesignHTMLJob, error) {
	j := &DesignHTMLJob{}
	err := row.Scan(&j.ID, &j.SessionID, &j.RequestKey, &j.Message, &j.Prompt, &j.SourceVersionID, &j.Status, &j.Error, &j.CreatedAt, &j.UpdatedAt, &j.Reasoning, &j.GenerationPhase, &j.GenerationModel)
	return j, err
}

func (r *Repository) UpdateDesignHTMLProgress(jobID, model, phase, reasoning string) error {
	_, err := r.db.Exec(`UPDATE design_html_jobs SET generation_model=?,generation_phase=?,reasoning=? WHERE id=? AND status='running'`, model, phase, reasoning, jobID)
	return err
}

const designHTMLVersionColumns = `id,session_id,job_id,version,parent_version_id,file_name,size_bytes,model,created_at`

func scanDesignHTMLVersion(row *sql.Row) (*DesignHTMLVersion, error) {
	i := &DesignHTMLVersion{}
	err := row.Scan(&i.ID, &i.SessionID, &i.JobID, &i.Version, &i.ParentVersionID, &i.FileName, &i.SizeBytes, &i.Model, &i.CreatedAt)
	return i, err
}

func designHTMLPrompt(v DesignVersion, message string, editing bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "方案标题：%s\n", v.Title)
	for n, q := range v.Questions {
		fmt.Fprintf(&b, "%d. %s\n学生填写：%s\n", n+1, q, v.Answers[n])
	}
	if message != "" {
		b.WriteString("\n本次要求：" + message)
	}
	return b.String()
}

func (r *Repository) EnqueueDesignHTML(planID, classID, studentID int, key, message string, sourceAnswers ...[]string) (*DesignHTMLJob, error) {
	key = strings.TrimSpace(key)
	if !designRequestKey.MatchString(key) {
		return nil, fmt.Errorf("请求标识无效")
	}
	message, err := normalizeDesignText(message, "补充或修改要求", designMessageLimit, false)
	if err != nil {
		return nil, err
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE design_plans SET title=title WHERE id=?`, planID); err != nil {
		return nil, err
	}
	if err = validateDesignStudent(tx, planID, classID, studentID); err != nil {
		return nil, err
	}
	if err = requireDesignOutputType(tx, planID, "html"); err != nil {
		return nil, err
	}
	stage, err := designStage(tx, planID, classID)
	if err != nil {
		return nil, err
	}
	if stage < 3 {
		return nil, fmt.Errorf("等待老师开启网页环节")
	}
	versions, err := loadDesignVersions(tx, planID, studentID)
	if err != nil {
		return nil, err
	}
	if len(versions) != 2 {
		return nil, fmt.Errorf("请先完成两版方案")
	}
	snapshot, _ := json.Marshal(versions[1])
	if _, err = tx.Exec(`INSERT OR IGNORE INTO design_html_sessions(plan_id,class_id,student_id,snapshot_json) VALUES(?,?,?,?)`, planID, classID, studentID, string(snapshot)); err != nil {
		return nil, err
	}
	var sessionID int
	var finalID string
	var savedSnapshot string
	if err = tx.QueryRow(`SELECT id,final_version_id,snapshot_json FROM design_html_sessions WHERE plan_id=? AND student_id=?`, planID, studentID).Scan(&sessionID, &finalID, &savedSnapshot); err != nil {
		return nil, err
	}
	if old, e := scanDesignHTMLJob(tx.QueryRow(`SELECT `+designHTMLJobColumns+` FROM design_html_jobs WHERE session_id=? AND request_key=?`, sessionID, key)); e == nil {
		return old, nil
	} else if e != sql.ErrNoRows {
		return nil, e
	}
	if finalID != "" {
		return nil, fmt.Errorf("最终作品已提交，不可继续生成")
	}
	var active bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM design_html_jobs WHERE session_id=? AND status IN ('queued','running'))`, sessionID).Scan(&active); err != nil {
		return nil, err
	}
	if active {
		return nil, fmt.Errorf("已有生成任务，请等待完成")
	}
	if err = checkDesignGenerationBudget(tx, studentID); err != nil {
		return nil, err
	}
	var source string
	err = tx.QueryRow(`SELECT id FROM design_html_versions WHERE session_id=? ORDER BY version DESC LIMIT 1`, sessionID).Scan(&source)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if source != "" && message == "" {
		return nil, fmt.Errorf("请填写网页修改要求")
	}
	var v DesignVersion
	if err = json.Unmarshal([]byte(savedSnapshot), &v); err != nil {
		return nil, err
	}
	if len(sourceAnswers) > 0 && sourceAnswers[0] != nil {
		if source != "" {
			return nil, fmt.Errorf("已有网页，请通过修改要求调整网页")
		}
		answers := sourceAnswers[0]
		if len(answers) != len(v.Questions) {
			return nil, fmt.Errorf("请完整填写用于生成网页的内容")
		}
		v.Answers, err = normalizeDesignAnswers(answers)
		if err != nil {
			return nil, err
		}
		snapshot, err = json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(`UPDATE design_html_sessions SET snapshot_json=? WHERE id=?`, string(snapshot), sessionID); err != nil {
			return nil, err
		}
	}
	if err = validateDesignSnapshot(v); err != nil {
		return nil, err
	}
	prompt := designHTMLPrompt(v, message, source != "")
	if _, err = normalizeDesignText(prompt, "生成要求总长度", designPromptLimit, true); err != nil {
		return nil, err
	}
	if message == "" {
		message = "按第二版方案生成网页。"
		if len(sourceAnswers) > 0 && sourceAnswers[0] != nil {
			message = "按确认内容生成网页。"
		}
	}
	j := &DesignHTMLJob{ID: designRandomID(), SessionID: sessionID, RequestKey: key, Message: message, Prompt: prompt, SourceVersionID: source, Status: "queued", CreatedAt: designNow(), UpdatedAt: designNow()}
	if _, err = tx.Exec(`INSERT INTO design_html_jobs(`+designHTMLJobColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, j.ID, j.SessionID, j.RequestKey, j.Message, j.Prompt, j.SourceVersionID, j.Status, "", j.CreatedAt, j.UpdatedAt, "", "", ""); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`INSERT INTO design_html_messages(session_id,job_id,role,content,created_at) VALUES(?,?,'user',?,?)`, sessionID, j.ID, message, j.CreatedAt); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return j, nil
}

func (r *Repository) RecoverDesignHTMLJobs() error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO design_html_messages(session_id,job_id,role,content,created_at) SELECT session_id,id,'assistant','生成中断，请点击重试。',? FROM design_html_jobs WHERE status='running'`, designNow()); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE design_html_jobs SET status='interrupted',error='生成中断，请点击重试。',updated_at=? WHERE status='running'`, designNow()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ClaimDesignHTMLJob() (*DesignHTMLJob, error) {
	return scanDesignHTMLJob(r.db.QueryRow(`UPDATE design_html_jobs SET status='running',updated_at=? WHERE id=(SELECT id FROM design_html_jobs WHERE status='queued' ORDER BY created_at,id LIMIT 1) AND status='queued' RETURNING `+designHTMLJobColumns, designNow()))
}

func (r *Repository) FailDesignHTMLJob(j *DesignHTMLJob, text string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE design_html_jobs SET status='failed',error=?,updated_at=? WHERE id=? AND status='running'`, text, designNow(), j.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("任务状态已变化")
	}
	if _, err = tx.Exec(`INSERT INTO design_html_messages(session_id,job_id,role,content,created_at) VALUES(?,?,'assistant',?,?)`, j.SessionID, j.ID, text, designNow()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) CompleteDesignHTMLJob(j *DesignHTMLJob, i *DesignHTMLVersion) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE design_html_jobs SET status='succeeded',updated_at=? WHERE id=? AND status='running'`, designNow(), j.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("任务状态已变化")
	}
	if err = tx.QueryRow(`SELECT COALESCE(MAX(version),0)+1 FROM design_html_versions WHERE session_id=?`, j.SessionID).Scan(&i.Version); err != nil {
		return err
	}
	i.SessionID = j.SessionID
	i.JobID = j.ID
	i.ParentVersionID = j.SourceVersionID
	i.CreatedAt = designNow()
	if _, err = tx.Exec(`INSERT INTO design_html_versions(`+designHTMLVersionColumns+`) VALUES(?,?,?,?,?,?,?,?,?)`, i.ID, i.SessionID, i.JobID, i.Version, i.ParentVersionID, i.FileName, i.SizeBytes, i.Model, i.CreatedAt); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO design_html_messages(session_id,job_id,role,content,version_id,created_at) VALUES(?,?,'assistant',?,?,?)`, j.SessionID, j.ID, fmt.Sprintf("已生成第%d版网页。", i.Version), i.ID, i.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) DesignHTMLState(planID, studentID, beforeVersion, beforeMessage int) (*DesignHTMLState, error) {
	s := &DesignHTMLState{Versions: []DesignHTMLVersion{}, Messages: []DesignHTMLMessage{}}
	var snapshot string
	err := r.db.QueryRow(`SELECT id,final_version_id,finalized_at,snapshot_json FROM design_html_sessions WHERE plan_id=? AND student_id=?`, planID, studentID).Scan(&s.SessionID, &s.FinalVersionID, &s.FinalizedAt, &snapshot)
	if err == sql.ErrNoRows {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(snapshot), &s.SourcePlan); err != nil {
		return nil, err
	}
	if err = r.db.QueryRow(`SELECT COUNT(*) FROM design_html_versions WHERE session_id=?`, s.SessionID).Scan(&s.VersionCount); err != nil {
		return nil, err
	}
	if s.FinalVersionID != "" {
		s.FinalVersion, err = scanDesignHTMLVersion(r.db.QueryRow(`SELECT `+designHTMLVersionColumns+` FROM design_html_versions WHERE id=? AND session_id=?`, s.FinalVersionID, s.SessionID))
		if err != nil {
			return nil, err
		}
	}
	s.LatestVersion, err = scanDesignHTMLVersion(r.db.QueryRow(`SELECT `+designHTMLVersionColumns+` FROM design_html_versions WHERE session_id=? ORDER BY version DESC LIMIT 1`, s.SessionID))
	if err == sql.ErrNoRows {
		s.LatestVersion = nil
	} else if err != nil {
		return nil, err
	}
	s.ActiveJob, err = scanDesignHTMLJob(r.db.QueryRow(`SELECT `+designHTMLJobColumns+` FROM design_html_jobs WHERE session_id=? AND status IN ('queued','running') LIMIT 1`, s.SessionID))
	if err == sql.ErrNoRows {
		s.ActiveJob = nil
	} else if err != nil {
		return nil, err
	}
	s.LastJob, err = scanDesignHTMLJob(r.db.QueryRow(`SELECT `+designHTMLJobColumns+` FROM design_html_jobs WHERE session_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, s.SessionID))
	if err == sql.ErrNoRows {
		s.LastJob = nil
	} else if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`SELECT `+designHTMLVersionColumns+` FROM design_html_versions WHERE session_id=? AND (?=0 OR version<?) ORDER BY version DESC LIMIT 21`, s.SessionID, beforeVersion, beforeVersion)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i DesignHTMLVersion
		if err = rows.Scan(&i.ID, &i.SessionID, &i.JobID, &i.Version, &i.ParentVersionID, &i.FileName, &i.SizeBytes, &i.Model, &i.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		s.Versions = append(s.Versions, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(s.Versions) > 20 {
		s.VersionsHasMore = true
		s.Versions = s.Versions[:20]
	}
	rows, err = r.db.Query(`SELECT m.id,m.job_id,m.role,m.content,m.version_id,j.status,m.created_at FROM design_html_messages m JOIN design_html_jobs j ON j.id=m.job_id WHERE m.session_id=? AND (?=0 OR m.id<?) ORDER BY m.id DESC LIMIT 41`, s.SessionID, beforeMessage, beforeMessage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m DesignHTMLMessage
		if err = rows.Scan(&m.ID, &m.JobID, &m.Role, &m.Content, &m.VersionID, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		s.Messages = append(s.Messages, m)
	}
	if len(s.Messages) > 40 {
		s.MessagesHasMore = true
		s.Messages = s.Messages[:40]
	}
	return s, rows.Err()
}

func (r *Repository) FinalizeDesignHTML(planID, classID, studentID int, versionID string) error {
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
	if err = requireDesignOutputType(tx, planID, "html"); err != nil {
		return err
	}
	var sid int
	var old string
	if err = tx.QueryRow(`SELECT id,final_version_id FROM design_html_sessions WHERE plan_id=? AND student_id=?`, planID, studentID).Scan(&sid, &old); err != nil {
		return fmt.Errorf("尚未生成网页")
	}
	if old != "" {
		if old == versionID {
			return nil
		}
		return fmt.Errorf("最终作品已提交，不可更换")
	}
	var active, owned bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM design_html_jobs WHERE session_id=? AND status IN ('queued','running')),EXISTS(SELECT 1 FROM design_html_versions WHERE session_id=? AND id=?)`, sid, sid, versionID).Scan(&active, &owned); err != nil {
		return err
	}
	if active {
		return fmt.Errorf("请等待当前生成任务完成")
	}
	if !owned {
		return fmt.Errorf("只能选择自己的成功网页")
	}
	if _, err = tx.Exec(`UPDATE design_html_sessions SET final_version_id=?,finalized_at=? WHERE id=?`, versionID, designNow(), sid); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) OwnedDesignHTMLVersion(planID, studentID int, versionID string) (*DesignHTMLVersion, error) {
	return scanDesignHTMLVersion(r.db.QueryRow(`SELECT `+designHTMLVersionColumns+` FROM design_html_versions WHERE id=? AND session_id=(SELECT id FROM design_html_sessions WHERE plan_id=? AND student_id=?)`, versionID, planID, studentID))
}

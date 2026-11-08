package v2

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const designImageModel = "doubao-seedream-5-0-flash-260915"

func designNow() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func designRandomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func initDesignImages(db *sql.DB) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS design_image_sessions(id INTEGER PRIMARY KEY AUTOINCREMENT,plan_id INTEGER NOT NULL REFERENCES design_plans(id),class_id INTEGER NOT NULL REFERENCES classes(id),student_id INTEGER NOT NULL REFERENCES students(id),snapshot_json TEXT NOT NULL,final_image_id TEXT NOT NULL DEFAULT '',finalized_at TEXT NOT NULL DEFAULT '',UNIQUE(plan_id,student_id))`,
		`CREATE TABLE IF NOT EXISTS design_image_jobs(id TEXT PRIMARY KEY,session_id INTEGER NOT NULL REFERENCES design_image_sessions(id),request_key TEXT NOT NULL,message TEXT NOT NULL,prompt TEXT NOT NULL,source_image_id TEXT NOT NULL DEFAULT '',status TEXT NOT NULL CHECK(status IN ('queued','running','succeeded','failed','interrupted')),error TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL,updated_at TEXT NOT NULL,UNIQUE(session_id,request_key))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_design_active_job ON design_image_jobs(session_id) WHERE status IN ('queued','running')`,
		`CREATE INDEX IF NOT EXISTS idx_design_job_queue ON design_image_jobs(status,created_at)`,
		`CREATE TABLE IF NOT EXISTS design_images(id TEXT PRIMARY KEY,session_id INTEGER NOT NULL REFERENCES design_image_sessions(id),job_id TEXT NOT NULL UNIQUE REFERENCES design_image_jobs(id),version INTEGER NOT NULL,parent_image_id TEXT NOT NULL DEFAULT '',file_name TEXT NOT NULL UNIQUE,width INTEGER NOT NULL,height INTEGER NOT NULL,model TEXT NOT NULL,created_at TEXT NOT NULL,UNIQUE(session_id,version))`,
		`CREATE TABLE IF NOT EXISTS design_image_messages(id INTEGER PRIMARY KEY AUTOINCREMENT,session_id INTEGER NOT NULL REFERENCES design_image_sessions(id),job_id TEXT NOT NULL REFERENCES design_image_jobs(id),role TEXT NOT NULL CHECK(role IN ('user','assistant')),content TEXT NOT NULL,image_id TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_design_message_session ON design_image_messages(session_id,id)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

type DesignImageJob struct {
	ID            string `json:"id"`
	SessionID     int    `json:"-"`
	RequestKey    string `json:"requestKey"`
	Message       string `json:"message"`
	Prompt        string `json:"-"`
	SourceImageID string `json:"sourceImageId"`
	Status        string `json:"status"`
	Error         string `json:"error"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}
type DesignImage struct {
	ID            string `json:"id"`
	SessionID     int    `json:"-"`
	JobID         string `json:"jobId"`
	Version       int    `json:"version"`
	ParentImageID string `json:"parentImageId"`
	FileName      string `json:"-"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Model         string `json:"model"`
	CreatedAt     string `json:"createdAt"`
}
type DesignImageMessage struct {
	ID        int    `json:"id"`
	JobID     string `json:"jobId"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	ImageID   string `json:"imageId"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}
type DesignImageState struct {
	SourcePlan      *DesignVersion       `json:"sourcePlan,omitempty"`
	SessionID       int                  `json:"sessionId"`
	FinalImageID    string               `json:"finalImageId"`
	FinalizedAt     string               `json:"finalizedAt"`
	ImageCount      int                  `json:"imageCount"`
	Images          []DesignImage        `json:"images"`
	Messages        []DesignImageMessage `json:"messages"`
	ImagesHasMore   bool                 `json:"imagesHasMore"`
	MessagesHasMore bool                 `json:"messagesHasMore"`
	LatestImage     *DesignImage         `json:"latestImage"`
	FinalImage      *DesignImage         `json:"finalImage"`
	ActiveJob       *DesignImageJob      `json:"activeJob"`
	LastJob         *DesignImageJob      `json:"lastJob"`
}

const designJobColumns = `id,session_id,request_key,message,prompt,source_image_id,status,error,created_at,updated_at`

func scanDesignJob(row *sql.Row) (*DesignImageJob, error) {
	j := &DesignImageJob{}
	err := row.Scan(&j.ID, &j.SessionID, &j.RequestKey, &j.Message, &j.Prompt, &j.SourceImageID, &j.Status, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

const designImageColumns = `id,session_id,job_id,version,parent_image_id,file_name,width,height,model,created_at`

func scanDesignImage(row *sql.Row) (*DesignImage, error) {
	i := &DesignImage{}
	err := row.Scan(&i.ID, &i.SessionID, &i.JobID, &i.Version, &i.ParentImageID, &i.FileName, &i.Width, &i.Height, &i.Model, &i.CreatedAt)
	return i, err
}

func designImagePrompt(v DesignVersion, message string, editing bool) string {
	var b strings.Builder
	if editing {
		b.WriteString("请根据本次修改要求编辑参考图片，保持未要求更改的文字、构图和元素。学生的最新修改要求优先于原方案。\n")
	} else {
		b.WriteString("请根据学生填写的方案生成一张完整作品，默认竖版。只呈现学生要求的标题与内容，不把问卷题干、填写说明、示例或版本标记印在图片上。中文清晰准确。\n")
	}
	b.WriteString("原方案：" + v.Title + "\n")
	for n, q := range v.Questions {
		fmt.Fprintf(&b, "%d. %s\n学生填写：%s\n", n+1, q, v.Answers[n])
	}
	if message != "" {
		b.WriteString("\n本次要求：" + message)
	}
	return b.String()
}

func (r *Repository) EnqueueDesignImage(planID, classID, studentID int, key, message string, sourceAnswers ...[]string) (*DesignImageJob, error) {
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
	if err = requireDesignOutputType(tx, planID, "image"); err != nil {
		return nil, err
	}
	stage, err := designStage(tx, planID, classID)
	if err != nil {
		return nil, err
	}
	if stage < 3 {
		return nil, fmt.Errorf("等待老师开启图片环节")
	}
	versions, err := loadDesignVersions(tx, planID, studentID)
	if err != nil {
		return nil, err
	}
	if len(versions) != 2 {
		return nil, fmt.Errorf("请先完成两版方案")
	}
	snapshot, _ := json.Marshal(versions[1])
	if _, err = tx.Exec(`INSERT OR IGNORE INTO design_image_sessions(plan_id,class_id,student_id,snapshot_json) VALUES(?,?,?,?)`, planID, classID, studentID, string(snapshot)); err != nil {
		return nil, err
	}
	var sessionID int
	var finalID string
	var savedSnapshot string
	if err = tx.QueryRow(`SELECT id,final_image_id,snapshot_json FROM design_image_sessions WHERE plan_id=? AND student_id=?`, planID, studentID).Scan(&sessionID, &finalID, &savedSnapshot); err != nil {
		return nil, err
	}
	if old, e := scanDesignJob(tx.QueryRow(`SELECT `+designJobColumns+` FROM design_image_jobs WHERE session_id=? AND request_key=?`, sessionID, key)); e == nil {
		return old, nil
	} else if e != sql.ErrNoRows {
		return nil, e
	}
	if finalID != "" {
		return nil, fmt.Errorf("最终作品已提交，不可继续生成")
	}
	var active bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM design_image_jobs WHERE session_id=? AND status IN ('queued','running'))`, sessionID).Scan(&active); err != nil {
		return nil, err
	}
	if active {
		return nil, fmt.Errorf("已有生成任务，请等待完成")
	}
	if err = checkDesignGenerationBudget(tx, studentID); err != nil {
		return nil, err
	}
	var source string
	err = tx.QueryRow(`SELECT id FROM design_images WHERE session_id=? ORDER BY version DESC LIMIT 1`, sessionID).Scan(&source)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if source != "" && message == "" {
		return nil, fmt.Errorf("请填写图片修改要求")
	}
	var v DesignVersion
	if err = json.Unmarshal([]byte(savedSnapshot), &v); err != nil {
		return nil, err
	}
	if len(sourceAnswers) > 0 && sourceAnswers[0] != nil {
		if source != "" {
			return nil, fmt.Errorf("已有图片，请通过修改要求调整图片")
		}
		answers := sourceAnswers[0]
		if len(answers) != len(v.Questions) {
			return nil, fmt.Errorf("请完整填写用于生成图片的内容")
		}
		v.Answers, err = normalizeDesignAnswers(answers)
		if err != nil {
			return nil, err
		}
		snapshot, err = json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(`UPDATE design_image_sessions SET snapshot_json=? WHERE id=?`, string(snapshot), sessionID); err != nil {
			return nil, err
		}
	}
	if err = validateDesignSnapshot(v); err != nil {
		return nil, err
	}
	prompt := designImagePrompt(v, message, source != "")
	if _, err = normalizeDesignText(prompt, "生成要求总长度", designPromptLimit, true); err != nil {
		return nil, err
	}
	if message == "" {
		message = "按第二版方案生成图片。"
		if len(sourceAnswers) > 0 && sourceAnswers[0] != nil {
			message = "按确认内容生成图片。"
		}
	}
	j := &DesignImageJob{ID: designRandomID(), SessionID: sessionID, RequestKey: key, Message: message, Prompt: prompt, SourceImageID: source, Status: "queued", CreatedAt: designNow(), UpdatedAt: designNow()}
	if _, err = tx.Exec(`INSERT INTO design_image_jobs(`+designJobColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?)`, j.ID, j.SessionID, j.RequestKey, j.Message, j.Prompt, j.SourceImageID, j.Status, "", j.CreatedAt, j.UpdatedAt); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`INSERT INTO design_image_messages(session_id,job_id,role,content,created_at) VALUES(?,?,'user',?,?)`, sessionID, j.ID, message, j.CreatedAt); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return j, nil
}

func (r *Repository) RecoverDesignImageJobs() error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO design_image_messages(session_id,job_id,role,content,created_at) SELECT session_id,id,'assistant','生成中断，请点击重试。',? FROM design_image_jobs WHERE status='running'`, designNow()); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE design_image_jobs SET status='interrupted',error='生成中断，请点击重试。',updated_at=? WHERE status='running'`, designNow()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ClaimDesignImageJob() (*DesignImageJob, error) {
	return scanDesignJob(r.db.QueryRow(`UPDATE design_image_jobs SET status='running',updated_at=? WHERE id=(SELECT id FROM design_image_jobs WHERE status='queued' ORDER BY created_at,id LIMIT 1) AND status='queued' RETURNING `+designJobColumns, designNow()))
}

func (r *Repository) FailDesignImageJob(j *DesignImageJob, text string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE design_image_jobs SET status='failed',error=?,updated_at=? WHERE id=? AND status='running'`, text, designNow(), j.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("任务状态已变化")
	}
	if _, err = tx.Exec(`INSERT INTO design_image_messages(session_id,job_id,role,content,created_at) VALUES(?,?,'assistant',?,?)`, j.SessionID, j.ID, text, designNow()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) CompleteDesignImageJob(j *DesignImageJob, i *DesignImage) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE design_image_jobs SET status='succeeded',updated_at=? WHERE id=? AND status='running'`, designNow(), j.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("任务状态已变化")
	}
	if err = tx.QueryRow(`SELECT COALESCE(MAX(version),0)+1 FROM design_images WHERE session_id=?`, j.SessionID).Scan(&i.Version); err != nil {
		return err
	}
	i.SessionID = j.SessionID
	i.JobID = j.ID
	i.ParentImageID = j.SourceImageID
	i.Model = designImageModel
	i.CreatedAt = designNow()
	if _, err = tx.Exec(`INSERT INTO design_images(`+designImageColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?)`, i.ID, i.SessionID, i.JobID, i.Version, i.ParentImageID, i.FileName, i.Width, i.Height, i.Model, i.CreatedAt); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO design_image_messages(session_id,job_id,role,content,image_id,created_at) VALUES(?,?,'assistant',?,?,?)`, j.SessionID, j.ID, fmt.Sprintf("已生成第%d版图片。", i.Version), i.ID, i.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) DesignImageState(planID, studentID, beforeVersion, beforeMessage int) (*DesignImageState, error) {
	s := &DesignImageState{Images: []DesignImage{}, Messages: []DesignImageMessage{}}
	var snapshot string
	err := r.db.QueryRow(`SELECT id,final_image_id,finalized_at,snapshot_json FROM design_image_sessions WHERE plan_id=? AND student_id=?`, planID, studentID).Scan(&s.SessionID, &s.FinalImageID, &s.FinalizedAt, &snapshot)
	if err == sql.ErrNoRows {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(snapshot), &s.SourcePlan); err != nil {
		return nil, err
	}
	if err = r.db.QueryRow(`SELECT COUNT(*) FROM design_images WHERE session_id=?`, s.SessionID).Scan(&s.ImageCount); err != nil {
		return nil, err
	}
	if s.FinalImageID != "" {
		s.FinalImage, err = scanDesignImage(r.db.QueryRow(`SELECT `+designImageColumns+` FROM design_images WHERE id=? AND session_id=?`, s.FinalImageID, s.SessionID))
		if err != nil {
			return nil, err
		}
	}
	s.LatestImage, err = scanDesignImage(r.db.QueryRow(`SELECT `+designImageColumns+` FROM design_images WHERE session_id=? ORDER BY version DESC LIMIT 1`, s.SessionID))
	if err == sql.ErrNoRows {
		s.LatestImage = nil
	} else if err != nil {
		return nil, err
	}
	s.ActiveJob, err = scanDesignJob(r.db.QueryRow(`SELECT `+designJobColumns+` FROM design_image_jobs WHERE session_id=? AND status IN ('queued','running') LIMIT 1`, s.SessionID))
	if err == sql.ErrNoRows {
		s.ActiveJob = nil
	} else if err != nil {
		return nil, err
	}
	s.LastJob, err = scanDesignJob(r.db.QueryRow(`SELECT `+designJobColumns+` FROM design_image_jobs WHERE session_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, s.SessionID))
	if err == sql.ErrNoRows {
		s.LastJob = nil
	} else if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`SELECT `+designImageColumns+` FROM design_images WHERE session_id=? AND (?=0 OR version<?) ORDER BY version DESC LIMIT 21`, s.SessionID, beforeVersion, beforeVersion)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var i DesignImage
		if err = rows.Scan(&i.ID, &i.SessionID, &i.JobID, &i.Version, &i.ParentImageID, &i.FileName, &i.Width, &i.Height, &i.Model, &i.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		s.Images = append(s.Images, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(s.Images) > 20 {
		s.ImagesHasMore = true
		s.Images = s.Images[:20]
	}
	rows, err = r.db.Query(`SELECT m.id,m.job_id,m.role,m.content,m.image_id,j.status,m.created_at FROM design_image_messages m JOIN design_image_jobs j ON j.id=m.job_id WHERE m.session_id=? AND (?=0 OR m.id<?) ORDER BY m.id DESC LIMIT 41`, s.SessionID, beforeMessage, beforeMessage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m DesignImageMessage
		if err = rows.Scan(&m.ID, &m.JobID, &m.Role, &m.Content, &m.ImageID, &m.Status, &m.CreatedAt); err != nil {
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

func (r *Repository) FinalizeDesignImage(planID, classID, studentID int, imageID string) error {
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
	var sid int
	var old string
	if err = tx.QueryRow(`SELECT id,final_image_id FROM design_image_sessions WHERE plan_id=? AND student_id=?`, planID, studentID).Scan(&sid, &old); err != nil {
		return fmt.Errorf("尚未生成图片")
	}
	if old != "" {
		if old == imageID {
			return nil
		}
		return fmt.Errorf("最终作品已提交，不可更换")
	}
	var active, owned bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM design_image_jobs WHERE session_id=? AND status IN ('queued','running')),EXISTS(SELECT 1 FROM design_images WHERE session_id=? AND id=?)`, sid, sid, imageID).Scan(&active, &owned); err != nil {
		return err
	}
	if active {
		return fmt.Errorf("请等待当前生成任务完成")
	}
	if !owned {
		return fmt.Errorf("只能选择自己的成功图片")
	}
	if _, err = tx.Exec(`UPDATE design_image_sessions SET final_image_id=?,finalized_at=? WHERE id=?`, imageID, designNow(), sid); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) OwnedDesignImage(planID, studentID int, imageID string) (*DesignImage, error) {
	return scanDesignImage(r.db.QueryRow(`SELECT `+designImageColumns+` FROM design_images WHERE id=? AND session_id=(SELECT id FROM design_image_sessions WHERE plan_id=? AND student_id=?)`, imageID, planID, studentID))
}

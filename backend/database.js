const Database = require('better-sqlite3');
const path = require('path');
const fs = require('fs');
const logger = require('./logger');

const dataDir = path.join(__dirname, '../data');
const dbPath = path.join(dataDir, 'quiz-system.db');

// 确保数据目录存在
if (!fs.existsSync(dataDir)) {
  fs.mkdirSync(dataDir, { recursive: true });
}

// 初始化数据库连接，开启WAL模式支持高并发
const db = new Database(dbPath, {
  verbose: (msg) => logger.debug('SQL:', msg)
});

// 开启WAL模式，提高并发性能和数据安全性
db.pragma('journal_mode = WAL');
db.pragma('synchronous = NORMAL');
db.pragma('foreign_keys = ON');
db.pragma('busy_timeout = 5000'); // 并发时等待5秒超时

// 初始化数据库表
function initDatabase() {
  try {
    // 0. 系统配置表
    db.exec(`
      CREATE TABLE IF NOT EXISTS system_config (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        key TEXT NOT NULL UNIQUE,
        value TEXT NOT NULL,
        updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
      )
    `);

    // 1. 课程表：存储12节课的信息
    db.exec(`
      CREATE TABLE IF NOT EXISTS courses (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        lesson_number INTEGER NOT NULL UNIQUE, -- 1-12课
        description TEXT,
        part2_guide TEXT, -- 第二部分答题指引，富文本HTML格式
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
      )
    `);
    
    // 兼容旧数据库，新增part2_guide字段
    try {
      db.prepare('ALTER TABLE courses ADD COLUMN part2_guide TEXT').run();
      logger.info('新增courses表part2_guide字段成功');
    } catch (e) {
      // 字段已存在忽略错误
      logger.debug('courses表part2_guide字段已存在，无需新增');
    }

    // 2. 班级名单表：每个课程下的班级学生名单
    db.exec(`
      CREATE TABLE IF NOT EXISTS class_lists (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        course_id INTEGER NOT NULL,
        class_name TEXT NOT NULL, -- 五年级（1）班等
        student_name TEXT NOT NULL,
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        UNIQUE(course_id, class_name, student_name),
        FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
      )
    `);

    // 3. 课程阶段表：每个课程每个班级当前处于哪个部分（1-4）
    db.exec(`
      CREATE TABLE IF NOT EXISTS course_stages (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        course_id INTEGER NOT NULL,
        class_name TEXT NOT NULL,
        current_stage INTEGER NOT NULL DEFAULT 1, -- 1-4部分
        updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE,
        UNIQUE(course_id, class_name)
      )
    `);

    // 4. 班级课程绑定表：每个班级绑定对应的课程
    db.exec(`
      CREATE TABLE IF NOT EXISTS class_course_bind (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        class_name TEXT NOT NULL UNIQUE,
        course_id INTEGER NOT NULL,
        updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
      )
    `);

    // 4. 题目表：存储每个课程每个部分的自定义题目
    db.exec(`
      CREATE TABLE IF NOT EXISTS questions (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        course_id INTEGER NOT NULL,
        part INTEGER NOT NULL, -- 1-4部分
        question_type TEXT NOT NULL, -- single(单选)/multi(多选)/text(填空)/judge(判断)
        question_text TEXT NOT NULL,
        options TEXT, -- JSON格式的选项数组
        correct_answer TEXT, -- 正确答案，客观题用
        explanation TEXT, -- 答案解析
        sort_order INTEGER NOT NULL DEFAULT 0, -- 排序
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
      )
    `);

    // 5. 学生问卷表：存储每个学生每节课的答题数据
    db.exec(`
      CREATE TABLE IF NOT EXISTS student_surveys (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        course_id INTEGER NOT NULL,
        student_id TEXT NOT NULL, -- 班级_姓名 唯一标识
        student_name TEXT NOT NULL,
        class_name TEXT NOT NULL,
        part1_answers TEXT, -- JSON格式第一部分答案
        part2_answer TEXT, -- 第二部分开放题答案
        part3_answers TEXT, -- JSON格式第三部分答案
        part3_score INTEGER, -- 第三部分得分
        part4_answers TEXT, -- JSON格式第四部分答案
        submitted_at TEXT, -- 全部提交时间
        created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
        UNIQUE(course_id, student_id),
        FOREIGN KEY (course_id) REFERENCES courses(id) ON DELETE CASCADE
      )
    `);

    // 6. 系统配置表
    db.exec(`
      CREATE TABLE IF NOT EXISTS system_config (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL,
        updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
      )
    `);

    // 初始化12节默认课程
    const courseCount = db.prepare('SELECT COUNT(*) as count FROM courses').get().count;
    if (courseCount === 0) {
      const insertCourse = db.prepare('INSERT INTO courses (name, lesson_number) VALUES (?, ?)');
      for (let i = 1; i <= 12; i++) {
        insertCourse.run(`第${i}节课`, i);
      }
      logger.info('初始化12节默认课程完成');
    }

    // 初始化默认当前课程为第1节课
    const currentCourse = db.prepare('SELECT value FROM system_config WHERE `key` = \'current_course_id\'').get();
    if (!currentCourse) {
      db.prepare('INSERT INTO system_config (`key`, value) VALUES (?, ?)').run('current_course_id', '1');
      logger.info('初始化当前课程为第1节课');
    }

    // 初始化默认题目
    initDefaultQuestions();

    logger.info('数据库初始化完成');
  } catch (error) {
    logger.error('数据库初始化失败:', error);
    throw error;
  }
}

// 初始化默认题目
function initDefaultQuestions() {
  const courses = db.prepare('SELECT id FROM courses').all();
  
  courses.forEach(course => {
    const questionCount = db.prepare('SELECT COUNT(*) as count FROM questions WHERE course_id = ?').get(course.id).count;
    if (questionCount > 0) return;

    const insertQuestion = db.prepare(`
      INSERT INTO questions (course_id, part, question_type, question_text, options, correct_answer, explanation, sort_order)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    `);

    // 第一部分：2道选择题
    insertQuestion.run(
      course.id, 1, 'single', '今天这节课，你觉得你能学会多少呢？0到5分，给自己打个分吧！',
      JSON.stringify(['0分 - 完全没把握', '1分 - 我觉得很难', '2分 - 可能会有点难', '3分 - 能学会一半吧', '4分 - 大部分能学会', '5分 - 我全都能学会！']),
      null, null, 1
    );

    insertQuestion.run(
      course.id, 1, 'multi', '为了达到学习效果，你打算怎么做？',
      JSON.stringify(['竖起耳朵认真听讲', '遇到不懂的查资料', '请教老师或同学', '边听边记笔记', '和同桌讨论', '其他']),
      null, null, 2
    );

    // 第二部分：开放题
    insertQuestion.run(
      course.id, 2, 'text', '人工智能可以代替人类做什么，不能做什么？请写下你的思考。',
      null, null, null, 1
    );

    // 第三部分：5道选择题，默认题
    insertQuestion.run(
      course.id, 3, 'single', '下面哪一项不是人工智能的三个核心要素？',
      JSON.stringify(['A. 数据', 'B. 算法', 'C. 规则', 'D. 算力']),
      'C', '人工智能的三个核心要素是数据、算法和算力。规则不是核心要素。', 1
    );

    insertQuestion.run(
      course.id, 3, 'single', '下面哪一项是让人工智能算得又快又猛的"火力"？',
      JSON.stringify(['A. 数据', 'B. 算法', 'C. 规则', 'D. 算力']),
      'D', '算力是让人工智能算得又快又猛的"火力"，它决定了人工智能的计算速度和能力。', 2
    );

    insertQuestion.run(
      course.id, 3, 'judge', '人工智能是人类制造的、能模仿人类智力和能力的一种技术。',
      JSON.stringify(['对', '错']),
      '对', '这个说法是正确的。人工智能是人类制造的、能模仿人类智力和能力的一种技术。', 3
    );

    insertQuestion.run(
      course.id, 3, 'judge', '学习累了的时候，可以让人工智能帮忙写作业。',
      JSON.stringify(['对', '错']),
      '错', '这个说法是错误的。我们应该自己完成作业，人工智能可以帮助我们学习，但不能代替我们写作业。', 4
    );

    insertQuestion.run(
      course.id, 3, 'judge', '人工智能是一种可以帮助我们学习的工具。',
      JSON.stringify(['对', '错']),
      '对', '这个说法是正确的。人工智能是一种可以帮助我们学习的工具，我们应该正确使用它。', 5
    );

    logger.info(`课程${course.id}默认题目初始化完成`);
  });
}

// 数据库操作方法
const dbOps = {
  // 课程相关
  getAllCourses: () => db.prepare('SELECT * FROM courses ORDER BY lesson_number').all(),
  getCourseById: (id) => db.prepare('SELECT * FROM courses WHERE id = ?').get(id),
  updateCourse: (id, name, description) => db.prepare(
    'UPDATE courses SET name = ?, description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?'
  ).run(name, description, id),
  // 第二部分答题指引
  getPart2Guide: (courseId) => {
    const res = db.prepare('SELECT part2_guide FROM courses WHERE id = ?').get(courseId);
    return res ? res.part2_guide || '' : '';
  },
  updatePart2Guide: (courseId, guideContent) => db.prepare(
    'UPDATE courses SET part2_guide = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?'
  ).run(guideContent, courseId),

  // 系统配置
  getCurrentCourseId: () => {
    const res = db.prepare('SELECT value FROM system_config WHERE `key` = \'current_course_id\'').get();
    return res ? parseInt(res.value) : 1;
  },
  setCurrentCourseId: (id) => db.prepare(
    'REPLACE INTO system_config (`key`, value, updated_at) VALUES (\'current_course_id\', ?, CURRENT_TIMESTAMP)'
  ).run(id),

  // 班级名单
  getClassList: (courseId, className) => db.prepare(
    'SELECT DISTINCT student_name FROM class_lists WHERE class_name = ? ORDER BY student_name'
  ).all(className),
  importClassList: (courseId, className, studentNames) => {
    const transaction = db.transaction((names) => {
      // 先删除旧名单
      db.prepare('DELETE FROM class_lists WHERE class_name = ?').run(className);
      // 插入新名单
      const insert = db.prepare('INSERT INTO class_lists (course_id, class_name, student_name) VALUES (?, ?, ?)');
      names.forEach(name => insert.run(courseId, className, name.trim()));
    });
    transaction(studentNames);
  },
  validateStudent: (courseId, className, studentName) => {
    const res = db.prepare(
      'SELECT 1 FROM class_lists WHERE class_name = ? AND student_name = ?'
    ).get(className, studentName.trim());
    return !!res;
  },
  getAllClasses: (courseId) => db.prepare(
    'SELECT DISTINCT class_name FROM class_lists  ORDER BY class_name'
  ).all(),

  // 课程阶段
  getCurrentStage: (courseId, className) => {
    if (className) {
      // 按班级维度查询
      const res = db.prepare('SELECT current_stage FROM course_stages WHERE course_id = ? AND class_name = ?').get(courseId, className);
      return res ? res.current_stage : 1;
    } else {
      // 兼容旧逻辑，查询该课程所有班级的最大阶段
      const res = db.prepare('SELECT MAX(current_stage) as max_stage FROM course_stages WHERE course_id = ?').get(courseId);
      return res && res.max_stage ? res.max_stage : 1;
    }
  },
  setCurrentStage: (courseId, stage, className) => {
    if (className) {
      // 按班级维度设置
      return db.prepare(
        'REPLACE INTO course_stages (course_id, class_name, current_stage, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)'
      ).run(courseId, className, stage);
    } else {
      // 兼容旧逻辑，设置该课程所有班级为同一阶段
      return db.prepare(
        'REPLACE INTO course_stages (course_id, class_name, current_stage, updated_at) VALUES (?, "common", ?, CURRENT_TIMESTAMP)'
      ).run(courseId, stage);
    }
  },

  // 班级课程绑定相关
  getClassBoundCourse: (className) => {
    const res = db.prepare('SELECT course_id FROM class_course_bind WHERE class_name = ?').get(className);
    return res ? res.course_id : 1; // 默认绑定第1节课
  },
  setClassBoundCourse: (className, courseId) => db.prepare(
    'REPLACE INTO class_course_bind (class_name, course_id, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)'
  ).run(className, courseId),

  // 题目相关
  getQuestionsByPart: (courseId, part) => db.prepare(
    'SELECT * FROM questions WHERE course_id = ? AND part = ? ORDER BY sort_order'
  ).all(courseId, part),
  updateQuestion: (id, questionText, options, correctAnswer, explanation) => db.prepare(
    'UPDATE questions SET question_text = ?, options = ?, correct_answer = ?, explanation = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?'
  ).run(questionText, options, correctAnswer, explanation, id),

  // 学生问卷
  saveStudentPart1: (courseId, studentId, studentName, className, answers) => db.prepare(
    'REPLACE INTO student_surveys (course_id, student_id, student_name, class_name, part1_answers, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)'
  ).run(courseId, studentId, studentName, className, JSON.stringify(answers)),
  
  saveStudentPart2: (courseId, studentId, answer) => db.prepare(
    'UPDATE student_surveys SET part2_answer = ?, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?'
  ).run(answer, courseId, studentId),
  
  saveStudentPart3: (courseId, studentId, answers, score) => db.prepare(
    'UPDATE student_surveys SET part3_answers = ?, part3_score = ?, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?'
  ).run(JSON.stringify(answers), score, courseId, studentId),
  
  saveStudentPart4: (courseId, studentId, answers) => db.prepare(
    'UPDATE student_surveys SET part4_answers = ?, submitted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE course_id = ? AND student_id = ?'
  ).run(JSON.stringify(answers), courseId, studentId),
  
  getStudentSurvey: (courseId, studentId) => {
    const res = db.prepare('SELECT * FROM student_surveys WHERE course_id = ? AND student_id = ?').get(courseId, studentId);
    if (res) {
      res.part1_answers = res.part1_answers ? JSON.parse(res.part1_answers) : null;
      res.part3_answers = res.part3_answers ? JSON.parse(res.part3_answers) : null;
      res.part4_answers = res.part4_answers ? JSON.parse(res.part4_answers) : null;
    }
    return res;
  },
  
  // 统计相关
  getPartCompletionStats: (courseId, part, className = null) => {
    let condition = '';
    if (part === 2) {
      // 第二部分字段是part2_answer（不带s）
      condition = 'part2_answer IS NOT NULL';
    } else {
      // 其他部分都是带s的partX_answers
      condition = `part${part}_answers IS NOT NULL`;
    }
    let sql = `
      SELECT 
        COUNT(*) as total,
        SUM(CASE WHEN ${condition} THEN 1 ELSE 0 END) as completed
      FROM student_surveys 
      WHERE course_id = ?
    `;
    const params = [courseId];
    if (className) {
      sql += ' AND class_name = ?';
      params.push(className);
    }
    return db.prepare(sql).get(...params);
  },
  
  getQuestionStats: (courseId, part, questionId) => {
    const question = db.prepare('SELECT * FROM questions WHERE id = ?').get(questionId);
    if (!question) return null;

    const answers = db.prepare(`
      SELECT part${part}_answers as answers
      FROM student_surveys 
      WHERE course_id = ? AND part${part}_answers IS NOT NULL
    `).all(courseId);

    const stats = {};
    JSON.parse(question.options).forEach(opt => {
      stats[opt] = 0;
    });

    answers.forEach(ans => {
      const studentAnswers = JSON.parse(ans.answers);
      const answer = studentAnswers[`q${question.sort_order}`];
      if (answer) stats[answer]++;
    });

    return {
      question: question.question_text,
      options: JSON.parse(question.options),
      stats: stats,
      total: answers.length
    };
  },
  
  // 获取学生所有历史课程的分数对比
  getStudentHistoryScores: (studentId) => {
    const sql = `
      SELECT c.name as course_name, c.lesson_number, s.part3_score, s.part1_answers
      FROM student_surveys s
      JOIN courses c ON s.course_id = c.id
      WHERE s.student_id = ? AND s.part3_score IS NOT NULL
      ORDER BY c.lesson_number ASC
    `;
    const res = db.prepare(sql).all(studentId);
    return res.map(item => ({
      courseName: item.course_name,
      lessonNumber: item.lesson_number,
      actualScore: item.part3_score,
      predictedScore: item.part1_answers ? JSON.parse(item.part1_answers).predictionScore || 0 : 0
    }));
  },

  getAllStudentSurveys: (courseId, className = null) => {
    let sql = 'SELECT * FROM student_surveys WHERE course_id = ?';
    const params = [courseId];
    if (className) {
      sql += ' AND class_name = ?';
      params.push(className);
    }
    const res = db.prepare(sql).all(...params);
    return res.map(s => ({
      ...s,
      part1_answers: s.part1_answers ? JSON.parse(s.part1_answers) : null,
      part3_answers: s.part3_answers ? JSON.parse(s.part3_answers) : null,
      part4_answers: s.part4_answers ? JSON.parse(s.part4_answers) : null
    }));
  }
};

module.exports = {
  initDatabase,
  db,
  ...dbOps
};

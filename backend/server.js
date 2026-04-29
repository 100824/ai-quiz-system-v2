const express = require('express');
const XLSX = require("xlsx");
const cors = require('cors');
const bodyParser = require('body-parser');
const path = require('path');
const logger = require('./logger');
const { initDatabase, getAllCourses, getCurrentCourseId, setCurrentCourseId, getClassList, importClassList, validateStudent, getAllClasses, getCurrentStage, setCurrentStage, getQuestionsByPart, updateQuestion, saveStudentPart1, saveStudentPart2, saveStudentPart3, saveStudentPart4, getStudentSurvey, getPartCompletionStats, getQuestionStats, getAllStudentSurveys, getClassBoundCourse, setClassBoundCourse, getStudentHistoryScores, getPart2Guide, updatePart2Guide, getClassTip, saveClassTip } = require('./database');

const app = express();
const PORT = process.env.PORT || 8080;

// 课堂提示语已持久化到数据库 class_tips 表

// 中间件
app.use(cors());
app.use(bodyParser.json({ limit: '10mb' }));
app.use(bodyParser.urlencoded({ extended: true }));

// 日志中间件
app.use((req, res, next) => {
  const start = Date.now();
  res.on('finish', () => {
    const duration = Date.now() - start;
    logger.info(`${req.method} ${req.path} ${res.statusCode} ${duration}ms`);
  });
  next();
});

// 静态文件托管前端
app.use(express.static(path.join(__dirname, '../frontend')));

// 初始化数据库
initDatabase();

// ==================== 教师端接口 ====================

// 获取所有课程
app.get('/api/teacher/courses', (req, res) => {
  try {
    const courses = getAllCourses();
    const currentCourseId = getCurrentCourseId();
    res.json({
      success: true,
      data: { courses, currentCourseId }
    });
  } catch (error) {
    logger.error('获取课程列表失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 切换当前课程
app.post('/api/teacher/current-course', (req, res) => {
  try {
    const { courseId } = req.body;
    setCurrentCourseId(courseId);
    // 新课程默认阶段为1
    setCurrentStage(courseId, 1);
    res.json({ success: true, message: '切换课程成功' });
  } catch (error) {
    logger.error('切换课程失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取班级名单
app.get('/api/teacher/class-list/:courseId/:className', (req, res) => {
  try {
    const { courseId, className } = req.params;
    const students = getClassList(parseInt(courseId), className);
    res.json({ success: true, data: { students } });
  } catch (error) {
    logger.error('获取班级名单失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 导入班级名单
app.post('/api/teacher/class-list', (req, res) => {
  try {
    const { courseId, className, studentNames } = req.body;
    importClassList(parseInt(courseId), className, studentNames);
    res.json({ success: true, message: '导入成功' });
  } catch (error) {
    logger.error('导入班级名单失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取所有班级
app.get('/api/teacher/classes/:courseId', (req, res) => {
  try {
    const { courseId } = req.params;
    const classes = getAllClasses(parseInt(courseId));
    res.json({ success: true, data: { classes } });
  } catch (error) {
    logger.error('获取班级列表失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取课程当前阶段
app.get('/api/teacher/stage/:courseId', (req, res) => {
  try {
    const { courseId } = req.params;
    const { className } = req.query;
    const stage = getCurrentStage(parseInt(courseId), className);
    res.json({ success: true, data: { stage } });
  } catch (error) {
    logger.error('获取课程阶段失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 设置课程阶段
app.post('/api/teacher/stage', (req, res) => {
  try {
    const { courseId, stage, className } = req.body;
    setCurrentStage(parseInt(courseId), parseInt(stage), className);
    res.json({ success: true, message: `已将${className || '所有班级'}切换到第${stage}部分` });
  } catch (error) {
    logger.error('设置课程阶段失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 绑定班级和课程
app.post('/api/teacher/bind-class-course', (req, res) => {
  try {
    const { className, courseId } = req.body;
    setClassBoundCourse(className, parseInt(courseId));
    res.json({ success: true, message: '班级课程绑定成功' });
  } catch (error) {
    logger.error('绑定班级课程失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 提交课堂提示语
app.post('/api/teacher/tip', (req, res) => {
  try {
    const { className, content } = req.body;
    // 只用班级名作为key，一个班级同一时间只上一个课，避免课程ID不匹配问题
    saveClassTip(className, content || '');
    res.json({ success: true, message: '提示语提交成功' });
  } catch (error) {
    logger.error('提交提示语失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取课堂提示语（教师端）
app.get('/api/teacher/tip', (req, res) => {
  try {
    const { className } = req.query;
    const content = getClassTip(className);
    res.json({ success: true, data: { content } });
  } catch (error) {
    logger.error('获取教师端提示语失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取某个部分的题目
app.get('/api/teacher/questions/:courseId/:part', (req, res) => {
  try {
    const { courseId, part } = req.params;
    const questions = getQuestionsByPart(parseInt(courseId), parseInt(part));
    res.json({ success: true, data: { questions } });
  } catch (error) {
    logger.error('获取题目失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 更新题目
app.post('/api/teacher/question/:id', (req, res) => {
  try {
    const { id } = req.params;
    const { questionText, options, correctAnswer, explanation } = req.body;
    updateQuestion(parseInt(id), questionText, options ? JSON.stringify(options) : null, correctAnswer, explanation);
    res.json({ success: true, message: '更新成功' });
  } catch (error) {
    logger.error('更新题目失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取第二部分答题指引
app.get('/api/teacher/part2-guide/:courseId', (req, res) => {
  try {
    const { courseId } = req.params;
    const guide = getPart2Guide(parseInt(courseId));
    res.json({ success: true, data: { guide } });
  } catch (error) {
    logger.error('获取答题指引失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 更新第二部分答题指引
app.post('/api/teacher/part2-guide/:courseId', (req, res) => {
  try {
    const { courseId } = req.params;
    const { content } = req.body;
    updatePart2Guide(parseInt(courseId), content);
    res.json({ success: true, message: '答题指引更新成功' });
  } catch (error) {
    logger.error('更新答题指引失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取统计数据
app.get('/api/teacher/stats/:courseId', (req, res) => {
  try {
    const { courseId } = req.params;
    const { className } = req.query;
    const stats = {};
    
    // 各部分完成情况
    for (let i = 1; i <= 4; i++) {
      stats[`part${i}`] = getPartCompletionStats(parseInt(courseId), i, className);
    }
    
    // 学生列表
    const students = getAllStudentSurveys(parseInt(courseId), className);
    stats.students = students;
    
    // ==================== 各部分填写情况统计 ====================
    // 第一部分统计
    stats.part1Stats = {
      predictionScoreDistribution: {}, // 预测得分分布
      learningMethodsDistribution: {}  // 学习方法选择分布
    };
    // 第二部分统计
    stats.part2Stats = {
      filledCount: 0,
      totalCount: students.length
    };
    // 第三部分统计
    const part3Questions = getQuestionsByPart(parseInt(courseId), 3);
    stats.part3Stats = {
      scoreDistribution: { 0: 0, 1: 0, 2: 0, 3: 0, 4: 0, 5: 0 }, // 得分分布
      questionCorrectRate: part3Questions.map((q, index) => ({
        questionId: q.id,
        questionText: q.question_text,
        sortOrder: index + 1,
        correctCount: 0,
        totalCount: 0,
        correctRate: 0
      })) // 每道题正确率
    };

    // 遍历所有学生统计
    students.forEach(s => {
      // 第一部分统计
      if (s.part1_answers) {
        // 预测得分
        const ps = s.part1_answers.predictionScore || 0;
        stats.part1Stats.predictionScoreDistribution[ps] = (stats.part1Stats.predictionScoreDistribution[ps] || 0) + 1;
        // 学习方法
        if (Array.isArray(s.part1_answers.learningMethods)) {
          s.part1_answers.learningMethods.forEach(method => {
            stats.part1Stats.learningMethodsDistribution[method] = (stats.part1Stats.learningMethodsDistribution[method] || 0) + 1;
          });
        }
        if (s.part1_answers.customMethod) {
          const customKey = `自定义: ${s.part1_answers.customMethod}`;
          stats.part1Stats.learningMethodsDistribution[customKey] = (stats.part1Stats.learningMethodsDistribution[customKey] || 0) + 1;
        }
      }

      // 第二部分统计
      if (s.part2_answer && s.part2_answer.trim()) {
        stats.part2Stats.filledCount += 1;
      }

      // 第三部分统计
      if (s.part3_score !== null) {
        const score = Math.min(5, Math.max(0, parseInt(s.part3_score) || 0));
        stats.part3Stats.scoreDistribution[score] += 1;
        
        // 统计每道题正确率
        if (s.part3_answers) {
          stats.part3Stats.questionCorrectRate.forEach((qStat, index) => {
            const qKey = `q${index + 1}`;
            if (s.part3_answers[qKey] !== undefined) {
              qStat.totalCount += 1;
              // 获取该题正确答案
              const question = part3Questions[index];
              if (s.part3_answers[qKey] === question.correct_answer) {
                qStat.correctCount += 1;
              }
              // 计算正确率
              qStat.correctRate = qStat.totalCount > 0 ? Math.round(qStat.correctCount / qStat.totalCount * 100) : 0;
            }
          });
        }
      }
    });
    
    res.json({ success: true, data: stats });
  } catch (error) {
    logger.error('获取统计数据失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 导出统计数据为Excel
app.get("/api/teacher/stats/:courseId/export", (req, res) => {
  try {
    const { courseId } = req.params;
    const { className } = req.query;
    const students = getAllStudentSurveys(parseInt(courseId), className);

    // 整理Excel数据
    // 统一截断函数：Excel单元格最大支持32767字符，预留足够余量
    const truncate = (str, max = 32000) => {
      if (!str) return str;
      str = String(str);
      return str.length > max ? `[内容过长已截断] ${str.slice(0, max)}` : str;
    };
    
    const excelData = students.map(s => {
      const status = s.part4_answers ? "已完成全部" : s.part3_answers ? "完成到第三部分" : s.part2_answer ? "完成到第二部分" : s.part1_answers ? "完成到第一部分" : "未开始";
      const part3Score = s.part3_score !== null ? `${s.part3_score}/5` : "未完成";
      
      // 第一部分字段拆分
      let predictionScore = "未提交";
      let learningMethods = "未提交";
      let customLearningMethod = "无";
      if (s.part1_answers) {
        predictionScore = truncate(`${s.part1_answers.predictionScore || 0}分`);
        learningMethods = truncate(Array.isArray(s.part1_answers.learningMethods) ? s.part1_answers.learningMethods.join("、") : "无");
        customLearningMethod = truncate(s.part1_answers.customMethod || "无");
      }
      
      // 第三部分字段拆分
      let q1Answer = "未提交", q2Answer = "未提交", q3Answer = "未提交", q4Answer = "未提交", q5Answer = "未提交";
      if (s.part3_answers) {
        q1Answer = s.part3_answers.q1 || "未答";
        q2Answer = s.part3_answers.q2 || "未答";
        q3Answer = s.part3_answers.q3 || "未答";
        q4Answer = s.part3_answers.q4 || "未答";
        q5Answer = s.part3_answers.q5 || "未答";
      }
      
      // 第四部分字段拆分
      let actualScore = "未提交", predictedScore = "未提交";
      let part4Q2Answer = "未提交", part4Q2Custom = "无";
      let part4Q3Answer = "未提交", part4Q3Custom = "无";
      if (s.part4_answers) {
        const part4 = typeof s.part4_answers === 'string' ? JSON.parse(s.part4_answers) : s.part4_answers;
        actualScore = part4.scoreCompare ? part4.scoreCompare.actual : "未提交";
        predictedScore = part4.scoreCompare ? part4.scoreCompare.predicted : "未提交";
        part4Q2Answer = part4.q2 && Array.isArray(part4.q2.answers) ? part4.q2.answers.join("、") : "未提交";
        part4Q2Custom = part4.q2 && part4.q2.custom ? part4.q2.custom : "无";
        part4Q3Answer = part4.q3 && Array.isArray(part4.q3.answers) ? part4.q3.answers.join("、") : "未提交";
        part4Q3Custom = part4.q3 && part4.q3.custom ? part4.q3.custom : "无";
      }
      
      return {
        "班级": truncate(s.class_name),
        "姓名": truncate(s.student_name),
        "完成状态": truncate(status),
        // 第一部分拆分字段
        "第一部分-预测得分": truncate(predictionScore),
        "第一部分-学习方法": truncate(learningMethods),
        "第一部分-自定义学习方法": truncate(customLearningMethod),
        // 第二部分
        "第二部分-开放题答案": truncate(s.part2_answer || "未提交"),
        // 第三部分拆分字段
        "第三部分-第1题答案": truncate(q1Answer),
        "第三部分-第2题答案": truncate(q2Answer),
        "第三部分-第3题答案": truncate(q3Answer),
        "第三部分-第4题答案": truncate(q4Answer),
        "第三部分-第5题答案": truncate(q5Answer),
        "第三部分-总得分": truncate(part3Score),
        // 第四部分拆分字段
        "第四部分-实际得分": truncate(actualScore),
        "第四部分-预测得分": truncate(predictedScore),
        "第四部分-第2题答案": truncate(part4Q2Answer),
        "第四部分-第2题自定义内容": truncate(part4Q2Custom),
        "第四部分-第3题答案": truncate(part4Q3Answer),
        "第四部分-第3题自定义内容": truncate(part4Q3Custom),
        "最后提交时间": truncate(s.updated_at || "未提交")
      };
    });

    // 生成Excel
    const ws = XLSX.utils.json_to_sheet(excelData);
    const wb = XLSX.utils.book_new();
    XLSX.utils.book_append_sheet(wb, ws, "答题记录");
    const buffer = XLSX.write(wb, { type: "buffer", bookType: "xlsx" });

    // 设置响应头
    const fileName = `答题记录_${className || "所有班级"}_${new Date().toISOString().slice(0,10)}.xlsx`;
    res.setHeader("Content-Disposition", `attachment; filename=${encodeURIComponent(fileName)}`);
    res.setHeader("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");
    res.send(buffer);
  } catch (error) {
    logger.error("导出统计数据失败:", error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// ==================== 学生端接口 ====================

// 验证学生
app.post('/api/student/validate', (req, res) => {
  try {
    const { courseId, className, studentName } = req.body;
    const cid = courseId ? parseInt(courseId) : getCurrentCourseId();
    const isValid = validateStudent(cid, className, studentName);
    
    res.json({
      success: true,
      data: {
        valid: isValid,
        message: isValid ? '验证成功' : '学生不在班级名单中，请联系老师',
        courseId: getClassBoundCourse(className) // 返回该班级绑定的课程ID
      }
    });
  } catch (error) {
    logger.error('验证学生失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取课堂提示语（必须放在/:studentId路由前面，避免被匹配为studentId）
app.get('/api/student/tip', (req, res) => {
  try {
    const { className } = req.query;
    const content = getClassTip(className);
    res.json({ success: true, data: { content } });
  } catch (error) {
    logger.error('获取提示语失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取学生问卷状态
app.get('/api/student/:studentId', (req, res) => {
  try {
    const { studentId } = req.params;
    const { courseId } = req.query;
    const cid = courseId ? parseInt(courseId) : getCurrentCourseId();
    
    const survey = getStudentSurvey(cid, studentId);
    // 拆分学生ID得到班级名（格式：班级名_学生名）
    const className = studentId.split('_').slice(0, -1).join('_');
    const currentStage = getCurrentStage(cid, className);
    const course = getAllCourses().find(c => c.id === cid);
    
    // 获取学生历史课程分数对比
    const historyScores = getStudentHistoryScores(studentId);
    
    res.json({
      success: true,
      data: {
        survey: survey || null, // 空的时候返回null，避免前端undefined
        currentStage,
        courseName: course ? course.name : '',
        lessonNumber: course ? course.lesson_number : 1,
        historyScores // 历史分数对比数据
      }
    });
  } catch (error) {
    logger.error('获取学生状态失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 获取题目
app.get('/api/student/questions/:part', (req, res) => {
  try {
    const { part } = req.params;
    const { courseId } = req.query;
    const cid = courseId ? parseInt(courseId) : getCurrentCourseId();
    
    const questions = getQuestionsByPart(cid, parseInt(part));
    res.json({ success: true, data: { questions } });
  } catch (error) {
    logger.error('获取题目失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 提交第一部分
app.post('/api/student/:studentId/part1', (req, res) => {
  try {
    const { studentId } = req.params;
    const { courseId, studentName, className, answers } = req.body;
    const cid = courseId ? parseInt(courseId) : getCurrentCourseId();
    
    saveStudentPart1(cid, studentId, studentName, className, answers);
    
    res.json({
      success: true,
      message: '第一部分提交成功',
      data: answers
    });
  } catch (error) {
    logger.error('提交第一部分失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 提交第二部分
app.post('/api/student/:studentId/part2', (req, res) => {
  try {
    const { studentId } = req.params;
    const { courseId, answer } = req.body;
    const cid = courseId ? parseInt(courseId) : getCurrentCourseId();
    
    saveStudentPart2(cid, studentId, answer);
    
    // 获取第二部分题目的解析
    const part2Questions = getQuestionsByPart(cid, 2);
    const explanation = part2Questions.length > 0 ? part2Questions[0].explanation : '';
    
    res.json({
      success: true,
      message: '第二部分提交成功',
      data: { 
        answer,
        explanation
      }
    });
  } catch (error) {
    logger.error('提交第二部分失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 提交第三部分
app.post('/api/student/:studentId/part3', (req, res) => {
  try {
    const { studentId } = req.params;
    const { courseId, answers } = req.body;
    const cid = courseId ? parseInt(courseId) : getCurrentCourseId();
    
    // 计算得分
    const questions = getQuestionsByPart(cid, 3);
    let score = 0;
    questions.forEach((q, index) => {
      if (answers[`q${index + 1}`] === q.correct_answer) {
        score++;
      }
    });
    
    saveStudentPart3(cid, studentId, answers, score);
    
    // 返回得分和解析
    const results = questions.map((q, index) => ({
      question: q.question_text,
      studentAnswer: answers[`q${index + 1}`],
      correctAnswer: q.correct_answer,
      explanation: q.explanation,
      isCorrect: answers[`q${index + 1}`] === q.correct_answer
    }));
    
    res.json({
      success: true,
      message: `第三部分提交成功，得分${score}/${questions.length}`,
      data: { score, total: questions.length, results }
    });
  } catch (error) {
    logger.error('提交第三部分失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 提交第四部分
app.post('/api/student/:studentId/part4', (req, res) => {
  try {
    const { studentId } = req.params;
    const { courseId, answers } = req.body;
    const cid = courseId ? parseInt(courseId) : getCurrentCourseId();
    
    saveStudentPart4(cid, studentId, answers);
    
    res.json({
      success: true,
      message: '问卷提交完成，感谢参与！'
    });
  } catch (error) {
    logger.error('提交第四部分失败:', error);
    res.status(500).json({ success: false, error: error.message });
  }
});

// 错误处理中间件
app.use((error, req, res, next) => {
  logger.error('请求处理失败:', error);
  res.status(500).json({ success: false, error: '服务器内部错误' });
});

// 404处理
app.use((req, res) => {
  res.status(404).json({ success: false, error: '接口不存在' });
});

// 启动服务
app.listen(PORT, '0.0.0.0', () => {
  logger.info(`服务启动成功，运行在 http://0.0.0.0:${PORT}`);
  logger.info(`学生端: http://0.0.0.0:${PORT}/student.html`);
  logger.info(`教师端: http://0.0.0.0:${PORT}/teacher.html`);
});

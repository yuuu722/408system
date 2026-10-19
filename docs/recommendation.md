# 当前学情与规则推荐

校准：2026-10-15。以 repository/personal.go、learning_platform.go、plans.go 和 domain/planner.go 的 PostgreSQL 正常路径为准；早期 ApplyMastery 行为加减分表不再作为当前产品说明。

## 有效结果与掌握度

资料或个人提供答案按后端判分；无资料答案时 AI 参考结果默认不计学情。用户确认正确/错误计入，不确定撤回。syncAssessmentEvents 替换本次测评贡献、追加结果变更账本并重算投影，保持原作答时间；晚确认旧试卷不会挤掉更新结果。

按题族最近有效作答统计：掌握度为有效题族正确率，薄弱度为 1 减正确率，置信度为题量规则投影（最多 1）；四科雷达未测为 null。题族为显式变式关系/确定性身份，不能识别所有语义重复题。

浏览、提问和完成任务只记录行为，不直接给掌握度加分。源码保留的早期 deltas/ApplyMastery 和不具备计划接口的 Store 回退不能当成当前正常测评评分规则。

## 每日计划与推荐

RecommendationService.Generate 在 PostgreSQL Store 上调用 GenerateTasks 和 PlanRecommendations，与学习任务共享一个计划入口。PlanCandidates 根据本人 ready 资源、有效题族记录、待测知识点、复习到期、偏好和人工难度排序；AI 估计难度不当成校准难度。

按北京时间首次生成冻结当日预算；之后修改 DailyMinutes 次日生效。已完成任务也占预算，重复生成只在剩余预算内补充，最多 20 项，无资源可为空。任务预计时间来自视频片段、人工分钟或默认估计，不是实际时长预测。

答错可立即补练，连续答对使用 1/3/7/14/30 天规则间隔。未完成历史任务可在新预算内承接，不无限累加。生成题默认 AI 判分，用户确认后才按自评计入。

## 尚未完成

未训练模型，未接入 FSRS/BKT/IRT/深度知识追踪，未校准能力概率与难度，也没有完整先修路径、自适应考试或教学效果证明。算法候选研究见 LEARNING_STRATEGY_RESEARCH.md；当前实现见 TASKS_3_5_IMPLEMENTATION.md。

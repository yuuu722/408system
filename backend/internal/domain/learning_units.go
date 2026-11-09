package domain

// A unit is a source-backed learning sequence, not a mastery prediction.
type LearningUnit struct {
	PointID, Name, Goal, Reason, Date, PlanID string
	ScheduledMinutes, EstimatedMinutes        int
	Knowledge                                 []Citation
	Materials                                 []Asset
	Example                                   *Asset
	Practice                                  []Asset
	Steps                                     []LearningUnitStep
	CheckAssessmentID                         string
}
type LearningUnitStep struct {
	Key, Title, ResourceKey, Notice string
	Minutes                         int
	Available, Completed            bool
}

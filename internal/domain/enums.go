package domain

// EntityStatus tracks the lifecycle of an entity in the pipeline.
type EntityStatus string

const (
	StatusPending  EntityStatus = "pending"
	StatusCreated  EntityStatus = "created"  // Created in PlentyONE
	StatusLinked   EntityStatus = "linked"   // Linked to parent entities
	StatusComplete EntityStatus = "complete" // All stages done
	StatusFailed   EntityStatus = "failed"
	StatusOrphaned EntityStatus = "orphaned" // Partially created, needs cleanup
	StatusSkipped  EntityStatus = "skipped"
)

// EntityType identifies what kind of entity a mapping refers to.
type EntityType string

const (
	EntityCategory  EntityType = "category"
	EntityAttribute EntityType = "attribute"
	EntityProperty  EntityType = "property"
	EntityProduct   EntityType = "product"
	EntityVariation EntityType = "variation"
	EntityImage     EntityType = "image"
	EntityText      EntityType = "text"
)

// PipelineStatus tracks the overall state of a pipeline run.
type PipelineStatus string

const (
	PipelinePending   PipelineStatus = "pending"
	PipelineRunning   PipelineStatus = "running"
	PipelinePaused    PipelineStatus = "paused"
	PipelineCompleted PipelineStatus = "completed"
	PipelineFailed    PipelineStatus = "failed"
)

// StageStatus tracks a single stage within a pipeline run.
type StageStatus string

const (
	StagePending   StageStatus = "pending"
	StageRunning   StageStatus = "running"
	StageCompleted StageStatus = "completed"
	StageFailed    StageStatus = "failed"
	StageSkipped   StageStatus = "skipped"
)

// StageName identifies the 6 pipeline stages.
type StageName string

const (
	StageCategories StageName = "categories"
	StageAttributes StageName = "attributes"
	StageProducts   StageName = "products"
	StageVariations StageName = "variations"
	StageImages     StageName = "images"
	StageTexts      StageName = "texts"
)

// JobType identifies the kind of job being executed.
type JobType string

const (
	JobGenerate JobType = "generate"
	JobPush     JobType = "push"
)

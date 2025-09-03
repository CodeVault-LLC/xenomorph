package routine

import (
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type RoutineTask interface {
	Name() string
	Run() error
}

type RoutineManager struct {
	tasks []RoutineTask
}

func NewRoutineManager() *RoutineManager {
	logger.L().Info("Initializing routine manager")
	return &RoutineManager{
		tasks: []RoutineTask{},
	}
}

func (rm *RoutineManager) Register(task RoutineTask) {
	logger.L().Info("Registering routine task", zap.String("task", task.Name()))
	rm.tasks = append(rm.tasks, task)
}

func (rm *RoutineManager) QueueTasks() {
	logger.L().Info("Starting routine tasks")

	for _, task := range rm.tasks {
		if err := task.Run(); err != nil {
			logger.L().Error("Task failed", zap.String("task", task.Name()), zap.Error(err))
		} else {
			logger.L().Info("Task completed successfully", zap.String("task", task.Name()))
		}
	}
}

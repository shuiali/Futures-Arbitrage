/**
 * MAKER Session Manager
 * Manages decomposed tasks and voting sessions
 */
import { DEFAULT_CONFIG, createVotingSession, addVoteToSession, createVote, calculateStats, generateTaskId, generateSubtaskId, suggestDecomposition, calculateKMin, } from './maker-core.js';
/** In-memory storage for active tasks and sessions */
class MakerSessionManager {
    tasks = new Map();
    votingSessions = new Map();
    config = DEFAULT_CONFIG;
    /**
     * Create a new decomposed task
     */
    createTask(originalTask, subtaskDescriptions) {
        const taskId = generateTaskId();
        const subtasks = subtaskDescriptions.map((desc, index) => ({
            id: generateSubtaskId(taskId, index + 1),
            description: desc,
            input: '',
            expectedOutputFormat: 'Provide a focused, concise implementation',
            dependencies: index > 0 ? [generateSubtaskId(taskId, index)] : [],
            status: 'pending',
        }));
        const task = {
            id: taskId,
            originalTask,
            subtasks,
            createdAt: new Date(),
            status: 'executing',
            currentStep: 0,
            totalSteps: subtasks.length,
        };
        this.tasks.set(taskId, task);
        return task;
    }
    /**
     * Auto-decompose a task using MAKER suggestions
     */
    autoDecomposeTask(taskDescription) {
        const suggestions = suggestDecomposition(taskDescription);
        return this.createTask(taskDescription, suggestions);
    }
    /**
     * Get a task by ID
     */
    getTask(taskId) {
        return this.tasks.get(taskId);
    }
    /**
     * Get all active tasks
     */
    getAllTasks() {
        return Array.from(this.tasks.values());
    }
    /**
     * Start voting for a subtask
     */
    startVoting(subtaskId) {
        const session = createVotingSession(subtaskId);
        this.votingSessions.set(subtaskId, session);
        // Update subtask status
        for (const task of this.tasks.values()) {
            const subtask = task.subtasks.find(s => s.id === subtaskId);
            if (subtask) {
                subtask.status = 'in-progress';
                break;
            }
        }
        return session;
    }
    /**
     * Submit a vote for a subtask
     */
    submitVote(subtaskId, response) {
        let session = this.votingSessions.get(subtaskId);
        if (!session) {
            session = this.startVoting(subtaskId);
        }
        const vote = createVote(subtaskId, response, this.config);
        const kMin = this.config.minVotesForConsensus;
        session = addVoteToSession(session, vote, kMin);
        this.votingSessions.set(subtaskId, session);
        const redFlags = vote.redFlags.map(f => `[${f.severity.toUpperCase()}] ${f.type}: ${f.description}`);
        if (session.isComplete) {
            this.completeSubtask(subtaskId, session);
        }
        return {
            session,
            isComplete: session.isComplete,
            winner: session.isComplete ? session.leadingResponse : null,
            redFlags,
        };
    }
    /**
     * Complete a subtask with the winning vote
     */
    completeSubtask(subtaskId, session) {
        for (const task of this.tasks.values()) {
            const subtask = task.subtasks.find(s => s.id === subtaskId);
            if (subtask && session.leadingResponse) {
                subtask.status = 'completed';
                subtask.result = {
                    output: session.leadingResponse,
                    votesReceived: session.leadingVoteCount,
                    votesRequired: this.config.minVotesForConsensus,
                    confidence: session.leadingVoteCount / session.responses.length,
                    wasRedFlagged: session.responses.some(v => !v.isValid),
                    attempts: session.responses.length,
                };
                task.currentStep++;
                if (task.currentStep >= task.totalSteps) {
                    task.status = 'completed';
                }
                break;
            }
        }
    }
    /**
     * Get voting session for a subtask
     */
    getVotingSession(subtaskId) {
        return this.votingSessions.get(subtaskId);
    }
    /**
     * Get statistics for a task
     */
    getTaskStats(taskId) {
        const task = this.tasks.get(taskId);
        if (!task)
            return null;
        return calculateStats(task);
    }
    /**
     * Calculate optimal k for a task
     */
    calculateOptimalK(totalSteps, estimatedSuccessRate = 0.95) {
        const result = calculateKMin(totalSteps, estimatedSuccessRate, this.config.targetSuccessProbability);
        return {
            kMin: result.kMin,
            expectedCost: result.expectedCost,
            successProbability: this.config.targetSuccessProbability,
        };
    }
    /**
     * Update configuration
     */
    updateConfig(newConfig) {
        this.config = { ...this.config, ...newConfig };
        return this.config;
    }
    /**
     * Get current configuration
     */
    getConfig() {
        return { ...this.config };
    }
    /**
     * Clear a completed or failed task
     */
    clearTask(taskId) {
        const task = this.tasks.get(taskId);
        if (!task)
            return false;
        // Clear associated voting sessions
        for (const subtask of task.subtasks) {
            this.votingSessions.delete(subtask.id);
        }
        return this.tasks.delete(taskId);
    }
    /**
     * Get the next pending subtask for a task
     */
    getNextSubtask(taskId) {
        const task = this.tasks.get(taskId);
        if (!task)
            return null;
        return task.subtasks.find(s => s.status === 'pending') || null;
    }
    /**
     * Reset a failed subtask for retry
     */
    resetSubtask(subtaskId) {
        for (const task of this.tasks.values()) {
            const subtask = task.subtasks.find(s => s.id === subtaskId);
            if (subtask) {
                subtask.status = 'pending';
                subtask.result = undefined;
                this.votingSessions.delete(subtaskId);
                return true;
            }
        }
        return false;
    }
}
// Export singleton instance
export const sessionManager = new MakerSessionManager();
//# sourceMappingURL=session-manager.js.map
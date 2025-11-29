/**
 * MAKER Session Manager
 * Manages decomposed tasks and voting sessions
 */
import { DecomposedTask, Subtask, VotingSession, MakerConfig, MakerStats } from './types.js';
/** In-memory storage for active tasks and sessions */
declare class MakerSessionManager {
    private tasks;
    private votingSessions;
    private config;
    /**
     * Create a new decomposed task
     */
    createTask(originalTask: string, subtaskDescriptions: string[]): DecomposedTask;
    /**
     * Auto-decompose a task using MAKER suggestions
     */
    autoDecomposeTask(taskDescription: string): DecomposedTask;
    /**
     * Get a task by ID
     */
    getTask(taskId: string): DecomposedTask | undefined;
    /**
     * Get all active tasks
     */
    getAllTasks(): DecomposedTask[];
    /**
     * Start voting for a subtask
     */
    startVoting(subtaskId: string): VotingSession;
    /**
     * Submit a vote for a subtask
     */
    submitVote(subtaskId: string, response: string): {
        session: VotingSession;
        isComplete: boolean;
        winner: string | null;
        redFlags: string[];
    };
    /**
     * Complete a subtask with the winning vote
     */
    private completeSubtask;
    /**
     * Get voting session for a subtask
     */
    getVotingSession(subtaskId: string): VotingSession | undefined;
    /**
     * Get statistics for a task
     */
    getTaskStats(taskId: string): MakerStats | null;
    /**
     * Calculate optimal k for a task
     */
    calculateOptimalK(totalSteps: number, estimatedSuccessRate?: number): {
        kMin: number;
        expectedCost: number;
        successProbability: number;
    };
    /**
     * Update configuration
     */
    updateConfig(newConfig: Partial<MakerConfig>): MakerConfig;
    /**
     * Get current configuration
     */
    getConfig(): MakerConfig;
    /**
     * Clear a completed or failed task
     */
    clearTask(taskId: string): boolean;
    /**
     * Get the next pending subtask for a task
     */
    getNextSubtask(taskId: string): Subtask | null;
    /**
     * Reset a failed subtask for retry
     */
    resetSubtask(subtaskId: string): boolean;
}
export declare const sessionManager: MakerSessionManager;
export {};
//# sourceMappingURL=session-manager.d.ts.map
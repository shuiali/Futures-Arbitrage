/**
 * MAKER Session Manager
 * Manages decomposed tasks and voting sessions
 */

import {
  DecomposedTask,
  Subtask,
  SubtaskResult,
  VotingSession,
  MakerConfig,
  MakerStats,
} from './types.js';
import {
  DEFAULT_CONFIG,
  createVotingSession,
  addVoteToSession,
  createVote,
  calculateStats,
  generateTaskId,
  generateSubtaskId,
  suggestDecomposition,
  calculateKMin,
} from './maker-core.js';

/** In-memory storage for active tasks and sessions */
class MakerSessionManager {
  private tasks: Map<string, DecomposedTask> = new Map();
  private votingSessions: Map<string, VotingSession> = new Map();
  private config: MakerConfig = DEFAULT_CONFIG;

  /**
   * Create a new decomposed task
   */
  createTask(originalTask: string, subtaskDescriptions: string[]): DecomposedTask {
    const taskId = generateTaskId();
    
    const subtasks: Subtask[] = subtaskDescriptions.map((desc, index) => ({
      id: generateSubtaskId(taskId, index + 1),
      description: desc,
      input: '',
      expectedOutputFormat: 'Provide a focused, concise implementation',
      dependencies: index > 0 ? [generateSubtaskId(taskId, index)] : [],
      status: 'pending',
    }));

    const task: DecomposedTask = {
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
  autoDecomposeTask(taskDescription: string): DecomposedTask {
    const suggestions = suggestDecomposition(taskDescription);
    return this.createTask(taskDescription, suggestions);
  }

  /**
   * Get a task by ID
   */
  getTask(taskId: string): DecomposedTask | undefined {
    return this.tasks.get(taskId);
  }

  /**
   * Get all active tasks
   */
  getAllTasks(): DecomposedTask[] {
    return Array.from(this.tasks.values());
  }

  /**
   * Start voting for a subtask
   */
  startVoting(subtaskId: string): VotingSession {
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
  submitVote(subtaskId: string, response: string): {
    session: VotingSession;
    isComplete: boolean;
    winner: string | null;
    redFlags: string[];
  } {
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
  private completeSubtask(subtaskId: string, session: VotingSession): void {
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
  getVotingSession(subtaskId: string): VotingSession | undefined {
    return this.votingSessions.get(subtaskId);
  }

  /**
   * Get statistics for a task
   */
  getTaskStats(taskId: string): MakerStats | null {
    const task = this.tasks.get(taskId);
    if (!task) return null;
    return calculateStats(task);
  }

  /**
   * Calculate optimal k for a task
   */
  calculateOptimalK(totalSteps: number, estimatedSuccessRate: number = 0.95): {
    kMin: number;
    expectedCost: number;
    successProbability: number;
  } {
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
  updateConfig(newConfig: Partial<MakerConfig>): MakerConfig {
    this.config = { ...this.config, ...newConfig };
    return this.config;
  }

  /**
   * Get current configuration
   */
  getConfig(): MakerConfig {
    return { ...this.config };
  }

  /**
   * Clear a completed or failed task
   */
  clearTask(taskId: string): boolean {
    const task = this.tasks.get(taskId);
    if (!task) return false;
    
    // Clear associated voting sessions
    for (const subtask of task.subtasks) {
      this.votingSessions.delete(subtask.id);
    }
    
    return this.tasks.delete(taskId);
  }

  /**
   * Get the next pending subtask for a task
   */
  getNextSubtask(taskId: string): Subtask | null {
    const task = this.tasks.get(taskId);
    if (!task) return null;
    
    return task.subtasks.find(s => s.status === 'pending') || null;
  }

  /**
   * Reset a failed subtask for retry
   */
  resetSubtask(subtaskId: string): boolean {
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

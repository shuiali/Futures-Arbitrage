/**
 * MAKER Core Logic
 * Implementation of the MAKER framework algorithms
 *
 * Based on: "Solving a Million-Step LLM Task with Zero Errors"
 * arXiv:2511.09030v1 [cs.AI] 12 Nov 2025
 */
import { MakerConfig, RedFlag, Vote, VotingSession, KMinCalculation, TaskAnalysis, Subtask, DecomposedTask, MakerStats } from './types.js';
/** Default MAKER configuration based on paper findings */
export declare const DEFAULT_CONFIG: MakerConfig;
/**
 * Calculate minimum k (votes) required for target success probability
 * Formula from paper: k_min = ⌈ln(t^(-m/s) - 1) / ln((1-p)/p)⌉
 */
export declare function calculateKMin(totalSteps: number, perStepSuccessRate: number, targetProbability?: number, stepsPerSubtask?: number): KMinCalculation;
/**
 * Calculate probability of correct selection with k votes
 * Formula: p(correct) = p^k / (p^k + (1-p)^k)
 */
export declare function calculateCorrectSelectionProbability(perStepSuccessRate: number, k: number): number;
/**
 * Calculate full task success probability
 * Formula: p_full = (1 + ((1-p)/p)^k)^(-s/m)
 */
export declare function calculateFullTaskSuccessProbability(totalSteps: number, perStepSuccessRate: number, k: number, stepsPerSubtask?: number): number;
/**
 * Detect red flags in a response
 */
export declare function detectRedFlags(response: string, config?: MakerConfig): RedFlag[];
/**
 * Estimate token count from text (rough approximation)
 */
export declare function estimateTokenCount(text: string): number;
/**
 * Create a vote from a response
 */
export declare function createVote(subtaskId: string, response: string, config?: MakerConfig): Vote;
/**
 * Hash a response for voting comparison
 * Normalizes whitespace and case for semantic comparison
 */
export declare function hashResponse(response: string): string;
/**
 * Create a new voting session for a subtask
 */
export declare function createVotingSession(subtaskId: string): VotingSession;
/**
 * Add a vote to a voting session
 * Implements "first-to-ahead-by-k" voting from the paper
 */
export declare function addVoteToSession(session: VotingSession, vote: Vote, k: number): VotingSession;
/**
 * Analyze a task and provide MAKER recommendations
 */
export declare function analyzeTask(taskDescription: string): TaskAnalysis;
/**
 * Generate decomposition suggestions for a coding task
 */
export declare function suggestDecomposition(taskDescription: string): string[];
/**
 * Format a subtask prompt for optimal LLM execution
 */
export declare function formatSubtaskPrompt(subtask: Subtask): string;
/**
 * Calculate MAKER statistics for a session
 */
export declare function calculateStats(task: DecomposedTask): MakerStats;
/**
 * Generate a unique task ID
 */
export declare function generateTaskId(): string;
/**
 * Generate a unique subtask ID
 */
export declare function generateSubtaskId(taskId: string, index: number): string;
//# sourceMappingURL=maker-core.d.ts.map
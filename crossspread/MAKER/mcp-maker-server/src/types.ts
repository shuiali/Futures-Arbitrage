/**
 * MAKER Framework Types
 * Based on the paper: "Solving a Million-Step LLM Task with Zero Errors"
 * arXiv:2511.09030v1 [cs.AI] 12 Nov 2025
 */

/** A single subtask in the decomposed task */
export interface Subtask {
  id: string;
  description: string;
  input: string;
  expectedOutputFormat: string;
  dependencies: string[];
  status: SubtaskStatus;
  result?: SubtaskResult;
}

export type SubtaskStatus = 'pending' | 'in-progress' | 'completed' | 'failed';

/** Result of a subtask execution */
export interface SubtaskResult {
  output: string;
  votesReceived: number;
  votesRequired: number;
  confidence: number;
  wasRedFlagged: boolean;
  attempts: number;
}

/** A decomposed task containing multiple subtasks */
export interface DecomposedTask {
  id: string;
  originalTask: string;
  subtasks: Subtask[];
  createdAt: Date;
  status: TaskStatus;
  currentStep: number;
  totalSteps: number;
}

export type TaskStatus = 'decomposing' | 'executing' | 'completed' | 'failed';

/** A single vote/response for a subtask */
export interface Vote {
  id: string;
  subtaskId: string;
  response: string;
  tokenCount: number;
  isValid: boolean;
  redFlags: RedFlag[];
  timestamp: Date;
}

/** Types of red flags that can be detected */
export interface RedFlag {
  type: RedFlagType;
  description: string;
  severity: 'low' | 'medium' | 'high';
}

export type RedFlagType = 
  | 'excessive-length'
  | 'format-error'
  | 'hedging-language'
  | 'todo-fixme'
  | 'generic-naming'
  | 'deprecated-api'
  | 'compilation-error'
  | 'uncertainty-markers';

/** Configuration for the MAKER framework */
export interface MakerConfig {
  /** Minimum votes required to reach consensus (k in the paper) */
  minVotesForConsensus: number;
  
  /** Maximum token count before flagging as too long */
  maxTokenThreshold: number;
  
  /** Target success probability (t in the paper) */
  targetSuccessProbability: number;
  
  /** Maximum attempts before giving up on a subtask */
  maxAttemptsPerSubtask: number;
  
  /** Enable red-flagging */
  enableRedFlagging: boolean;
  
  /** Red flag patterns to detect */
  redFlagPatterns: RedFlagPattern[];
}

export interface RedFlagPattern {
  type: RedFlagType;
  pattern: RegExp | ((response: string) => boolean);
  severity: 'low' | 'medium' | 'high';
}

/** Statistics for a MAKER execution session */
export interface MakerStats {
  totalSubtasks: number;
  completedSubtasks: number;
  failedSubtasks: number;
  totalVotes: number;
  redFlaggedVotes: number;
  averageVotesPerSubtask: number;
  estimatedSuccessRate: number;
  estimatedCost: number;
}

/** Voting session for a subtask */
export interface VotingSession {
  subtaskId: string;
  votes: Map<string, number>; // response hash -> vote count
  responses: Vote[];
  leadingResponse: string | null;
  leadingVoteCount: number;
  isComplete: boolean;
}

/** Result of calculating minimum k for voting */
export interface KMinCalculation {
  kMin: number;
  totalSteps: number;
  perStepSuccessRate: number;
  targetProbability: number;
  expectedCost: number;
}

/** Decomposition strategy hints */
export interface DecompositionHint {
  taskType: string;
  suggestedGranularity: 'fine' | 'medium' | 'coarse';
  maxLinesPerSubtask: number;
  patterns: string[];
}

/** Task analysis result */
export interface TaskAnalysis {
  complexity: 'low' | 'medium' | 'high' | 'very-high';
  estimatedSteps: number;
  suggestedKMin: number;
  estimatedCost: number;
  decompositionStrategy: string;
  warnings: string[];
}

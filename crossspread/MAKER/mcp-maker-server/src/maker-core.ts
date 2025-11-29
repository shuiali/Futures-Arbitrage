/**
 * MAKER Core Logic
 * Implementation of the MAKER framework algorithms
 * 
 * Based on: "Solving a Million-Step LLM Task with Zero Errors"
 * arXiv:2511.09030v1 [cs.AI] 12 Nov 2025
 */

import {
  MakerConfig,
  RedFlag,
  RedFlagType,
  Vote,
  VotingSession,
  KMinCalculation,
  TaskAnalysis,
  Subtask,
  DecomposedTask,
  MakerStats,
} from './types.js';
import { createHash } from 'crypto';

/** Default MAKER configuration based on paper findings */
export const DEFAULT_CONFIG: MakerConfig = {
  minVotesForConsensus: 3,
  maxTokenThreshold: 750, // From paper: errors spike after ~700 tokens
  targetSuccessProbability: 0.95,
  maxAttemptsPerSubtask: 10,
  enableRedFlagging: true,
  redFlagPatterns: [
    {
      type: 'excessive-length',
      pattern: (r: string) => r.length > 3000, // ~750 tokens
      severity: 'high',
    },
    {
      type: 'hedging-language',
      pattern: /\b(maybe|perhaps|might|could be|not sure|I think|possibly)\b/gi,
      severity: 'medium',
    },
    {
      type: 'todo-fixme',
      pattern: /\b(TODO|FIXME|XXX|HACK|BUG)\b/g,
      severity: 'high',
    },
    {
      type: 'generic-naming',
      pattern: /\b(data|result|temp|tmp|foo|bar|baz|test)\s*[=:]/g,
      severity: 'low',
    },
    {
      type: 'uncertainty-markers',
      pattern: /\.\.\.|etc\.|\?\?|!!!/g,
      severity: 'medium',
    },
  ],
};

/**
 * Calculate minimum k (votes) required for target success probability
 * Formula from paper: k_min = ⌈ln(t^(-m/s) - 1) / ln((1-p)/p)⌉
 */
export function calculateKMin(
  totalSteps: number,
  perStepSuccessRate: number,
  targetProbability: number = 0.95,
  stepsPerSubtask: number = 1 // m=1 for maximal decomposition
): KMinCalculation {
  const p = perStepSuccessRate;
  const t = targetProbability;
  const s = totalSteps;
  const m = stepsPerSubtask;
  
  // k_min = ⌈ln(t^(-m/s) - 1) / ln((1-p)/p)⌉
  const numerator = Math.log(Math.pow(t, -m / s) - 1);
  const denominator = Math.log((1 - p) / p);
  
  let kMin = Math.ceil(numerator / denominator);
  
  // Ensure minimum of 1
  kMin = Math.max(1, kMin);
  
  // Expected cost: Θ(s ln s) for m=1
  const expectedCost = s * Math.log(s) * kMin;
  
  return {
    kMin,
    totalSteps: s,
    perStepSuccessRate: p,
    targetProbability: t,
    expectedCost,
  };
}

/**
 * Calculate probability of correct selection with k votes
 * Formula: p(correct) = p^k / (p^k + (1-p)^k)
 */
export function calculateCorrectSelectionProbability(
  perStepSuccessRate: number,
  k: number
): number {
  const p = perStepSuccessRate;
  const pK = Math.pow(p, k);
  const oneMinusPK = Math.pow(1 - p, k);
  return pK / (pK + oneMinusPK);
}

/**
 * Calculate full task success probability
 * Formula: p_full = (1 + ((1-p)/p)^k)^(-s/m)
 */
export function calculateFullTaskSuccessProbability(
  totalSteps: number,
  perStepSuccessRate: number,
  k: number,
  stepsPerSubtask: number = 1
): number {
  const p = perStepSuccessRate;
  const ratio = Math.pow((1 - p) / p, k);
  return Math.pow(1 + ratio, -totalSteps / stepsPerSubtask);
}

/**
 * Detect red flags in a response
 */
export function detectRedFlags(
  response: string,
  config: MakerConfig = DEFAULT_CONFIG
): RedFlag[] {
  if (!config.enableRedFlagging) {
    return [];
  }
  
  const flags: RedFlag[] = [];
  
  for (const pattern of config.redFlagPatterns) {
    let isMatch = false;
    
    if (typeof pattern.pattern === 'function') {
      isMatch = pattern.pattern(response);
    } else {
      isMatch = pattern.pattern.test(response);
      // Reset regex lastIndex
      pattern.pattern.lastIndex = 0;
    }
    
    if (isMatch) {
      flags.push({
        type: pattern.type,
        description: getRedFlagDescription(pattern.type),
        severity: pattern.severity,
      });
    }
  }
  
  return flags;
}

function getRedFlagDescription(type: RedFlagType): string {
  const descriptions: Record<RedFlagType, string> = {
    'excessive-length': 'Response exceeds recommended token limit (>750 tokens). LLMs tend to over-analyze when confused.',
    'format-error': 'Response has formatting errors indicating reasoning confusion.',
    'hedging-language': 'Response contains hedging language suggesting uncertainty.',
    'todo-fixme': 'Response contains TODO/FIXME markers indicating incomplete solution.',
    'generic-naming': 'Response uses generic variable names suggesting lack of understanding.',
    'deprecated-api': 'Response uses deprecated APIs from stale training data.',
    'compilation-error': 'Response would not compile/type-check.',
    'uncertainty-markers': 'Response contains ellipsis or other uncertainty markers.',
  };
  return descriptions[type];
}

/**
 * Estimate token count from text (rough approximation)
 */
export function estimateTokenCount(text: string): number {
  // Rough estimate: ~4 characters per token for English text
  return Math.ceil(text.length / 4);
}

/**
 * Create a vote from a response
 */
export function createVote(
  subtaskId: string,
  response: string,
  config: MakerConfig = DEFAULT_CONFIG
): Vote {
  const tokenCount = estimateTokenCount(response);
  const redFlags = detectRedFlags(response, config);
  
  // Check token threshold
  if (tokenCount > config.maxTokenThreshold) {
    redFlags.push({
      type: 'excessive-length',
      description: `Response has ${tokenCount} tokens, exceeding threshold of ${config.maxTokenThreshold}`,
      severity: 'high',
    });
  }
  
  // A vote is valid only if it has no high-severity red flags
  const hasHighSeverityFlags = redFlags.some(f => f.severity === 'high');
  
  return {
    id: createHash('sha256').update(`${subtaskId}-${Date.now()}-${Math.random()}`).digest('hex').slice(0, 16),
    subtaskId,
    response,
    tokenCount,
    isValid: !hasHighSeverityFlags,
    redFlags,
    timestamp: new Date(),
  };
}

/**
 * Hash a response for voting comparison
 * Normalizes whitespace and case for semantic comparison
 */
export function hashResponse(response: string): string {
  const normalized = response
    .trim()
    .toLowerCase()
    .replace(/\s+/g, ' ');
  return createHash('sha256').update(normalized).digest('hex');
}

/**
 * Create a new voting session for a subtask
 */
export function createVotingSession(subtaskId: string): VotingSession {
  return {
    subtaskId,
    votes: new Map(),
    responses: [],
    leadingResponse: null,
    leadingVoteCount: 0,
    isComplete: false,
  };
}

/**
 * Add a vote to a voting session
 * Implements "first-to-ahead-by-k" voting from the paper
 */
export function addVoteToSession(
  session: VotingSession,
  vote: Vote,
  k: number
): VotingSession {
  // Only count valid (non-red-flagged) votes
  if (!vote.isValid) {
    session.responses.push(vote);
    return session;
  }
  
  const responseHash = hashResponse(vote.response);
  const currentCount = session.votes.get(responseHash) || 0;
  const newCount = currentCount + 1;
  session.votes.set(responseHash, newCount);
  session.responses.push(vote);
  
  // Update leading response
  if (newCount > session.leadingVoteCount) {
    session.leadingVoteCount = newCount;
    session.leadingResponse = vote.response;
  }
  
  // Check if we have a winner (ahead by k)
  let maxOtherCount = 0;
  for (const [hash, count] of session.votes) {
    if (hash !== responseHash && count > maxOtherCount) {
      maxOtherCount = count;
    }
  }
  
  if (session.leadingVoteCount >= k + maxOtherCount) {
    session.isComplete = true;
  }
  
  return session;
}

/**
 * Analyze a task and provide MAKER recommendations
 */
export function analyzeTask(taskDescription: string): TaskAnalysis {
  const words = taskDescription.split(/\s+/).length;
  const hasCodeKeywords = /\b(function|class|component|api|endpoint|database|service)\b/i.test(taskDescription);
  const hasMultipleSteps = /\b(and|then|also|after|before|finally)\b/gi.test(taskDescription);
  
  // Estimate complexity
  let complexity: TaskAnalysis['complexity'] = 'low';
  let estimatedSteps = 1;
  
  if (words > 50 || hasMultipleSteps) {
    complexity = 'medium';
    estimatedSteps = Math.ceil(words / 20);
  }
  
  if (words > 150 || (hasCodeKeywords && hasMultipleSteps)) {
    complexity = 'high';
    estimatedSteps = Math.ceil(words / 10);
  }
  
  if (words > 300) {
    complexity = 'very-high';
    estimatedSteps = Math.ceil(words / 5);
  }
  
  // Calculate suggested k_min assuming 0.95 success rate per step
  const kCalc = calculateKMin(estimatedSteps, 0.95);
  
  const warnings: string[] = [];
  
  if (complexity === 'very-high') {
    warnings.push('Task is very complex. Consider breaking it down before using MAKER.');
  }
  
  if (!hasCodeKeywords && words > 100) {
    warnings.push('Task may be too abstract. Try to be more specific about expected outputs.');
  }
  
  if (estimatedSteps > 100) {
    warnings.push(`Estimated ${estimatedSteps} steps. Ensure each subtask has clear success criteria.`);
  }
  
  return {
    complexity,
    estimatedSteps,
    suggestedKMin: kCalc.kMin,
    estimatedCost: kCalc.expectedCost,
    decompositionStrategy: getDecompositionStrategy(complexity),
    warnings,
  };
}

function getDecompositionStrategy(complexity: TaskAnalysis['complexity']): string {
  const strategies: Record<typeof complexity, string> = {
    'low': 'Single-step execution should suffice. Use voting with k=2 for verification.',
    'medium': 'Break into 3-5 subtasks. Each subtask should produce one concrete output.',
    'high': 'Use maximal decomposition. Each subtask should be ~20-30 lines of code max.',
    'very-high': 'Apply recursive decomposition. First break into modules, then into functions, then into individual operations.',
  };
  return strategies[complexity];
}

/**
 * Generate decomposition suggestions for a coding task
 */
export function suggestDecomposition(taskDescription: string): string[] {
  const suggestions: string[] = [];
  
  // Pattern matching for common task types
  if (/\b(api|endpoint|rest|graphql)\b/i.test(taskDescription)) {
    suggestions.push('1. Define request/response types');
    suggestions.push('2. Create input validation function');
    suggestions.push('3. Implement core business logic function');
    suggestions.push('4. Create error handling wrapper');
    suggestions.push('5. Compose into endpoint handler');
    suggestions.push('6. Write unit tests for each function');
  } else if (/\b(component|react|vue|angular)\b/i.test(taskDescription)) {
    suggestions.push('1. Define prop types/interface');
    suggestions.push('2. Create state management logic');
    suggestions.push('3. Implement event handlers');
    suggestions.push('4. Create render logic (JSX/template)');
    suggestions.push('5. Add styling');
    suggestions.push('6. Write component tests');
  } else if (/\b(function|algorithm|compute|calculate)\b/i.test(taskDescription)) {
    suggestions.push('1. Define input/output types');
    suggestions.push('2. Handle edge cases (null, empty, bounds)');
    suggestions.push('3. Implement core algorithm');
    suggestions.push('4. Add input validation');
    suggestions.push('5. Write unit tests');
  } else if (/\b(class|service|module)\b/i.test(taskDescription)) {
    suggestions.push('1. Define interface/contract');
    suggestions.push('2. Create constructor and initialization');
    suggestions.push('3. Implement each method individually');
    suggestions.push('4. Add error handling');
    suggestions.push('5. Write integration tests');
  } else {
    // Generic decomposition
    suggestions.push('1. Define types/interfaces for inputs and outputs');
    suggestions.push('2. Break into smallest logical functions');
    suggestions.push('3. Implement each function with clear single responsibility');
    suggestions.push('4. Add validation and error handling');
    suggestions.push('5. Compose functions into final solution');
    suggestions.push('6. Write tests for each component');
  }
  
  return suggestions;
}

/**
 * Format a subtask prompt for optimal LLM execution
 */
export function formatSubtaskPrompt(subtask: Subtask): string {
  return `## Subtask: ${subtask.description}

### Input
${subtask.input}

### Expected Output Format
${subtask.expectedOutputFormat}

### Instructions
- Focus ONLY on this specific subtask
- Keep response concise and focused
- Do not include explanations unless specifically requested
- Ensure output matches the expected format exactly
- If unsure, prefer simpler solutions

### Your Response:`;
}

/**
 * Calculate MAKER statistics for a session
 */
export function calculateStats(task: DecomposedTask): MakerStats {
  const completed = task.subtasks.filter(s => s.status === 'completed');
  const failed = task.subtasks.filter(s => s.status === 'failed');
  
  let totalVotes = 0;
  let redFlaggedVotes = 0;
  
  for (const subtask of task.subtasks) {
    if (subtask.result) {
      totalVotes += subtask.result.attempts;
      if (subtask.result.wasRedFlagged) {
        redFlaggedVotes++;
      }
    }
  }
  
  const avgVotes = task.subtasks.length > 0 ? totalVotes / task.subtasks.length : 0;
  const successRate = task.subtasks.length > 0 ? completed.length / task.subtasks.length : 0;
  
  return {
    totalSubtasks: task.subtasks.length,
    completedSubtasks: completed.length,
    failedSubtasks: failed.length,
    totalVotes,
    redFlaggedVotes,
    averageVotesPerSubtask: avgVotes,
    estimatedSuccessRate: successRate,
    estimatedCost: totalVotes * 0.001, // Rough cost estimate
  };
}

/**
 * Generate a unique task ID
 */
export function generateTaskId(): string {
  return `task-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

/**
 * Generate a unique subtask ID
 */
export function generateSubtaskId(taskId: string, index: number): string {
  return `${taskId}-step-${index.toString().padStart(4, '0')}`;
}

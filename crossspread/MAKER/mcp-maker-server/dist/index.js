#!/usr/bin/env node
/**
 * MAKER MCP Server
 *
 * An MCP server implementing the MAKER framework for massively decomposed
 * agentic processes. Based on the paper "Solving a Million-Step LLM Task
 * with Zero Errors" (arXiv:2511.09030v1).
 *
 * This server provides tools for:
 * - Task decomposition
 * - Voting-based error correction
 * - Red-flag detection
 * - Session management
 */
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { CallToolRequestSchema, ListToolsRequestSchema, ListPromptsRequestSchema, GetPromptRequestSchema, } from '@modelcontextprotocol/sdk/types.js';
import { analyzeTask, suggestDecomposition, detectRedFlags, calculateKMin, calculateFullTaskSuccessProbability, DEFAULT_CONFIG, formatSubtaskPrompt, } from './maker-core.js';
import { sessionManager } from './session-manager.js';
// Create the MCP server
const server = new Server({
    name: 'mcp-maker-server',
    version: '1.0.0',
}, {
    capabilities: {
        tools: {},
        prompts: {},
    },
});
// ============================================================================
// Tool Definitions
// ============================================================================
const TOOLS = [
    {
        name: 'maker_analyze_task',
        description: `Analyze a task to determine complexity and get MAKER recommendations.
Returns: complexity level, estimated steps, suggested k_min, decomposition strategy, and warnings.
Use this BEFORE starting any complex coding task.`,
        inputSchema: {
            type: 'object',
            properties: {
                task: {
                    type: 'string',
                    description: 'The task description to analyze',
                },
            },
            required: ['task'],
        },
    },
    {
        name: 'maker_decompose_task',
        description: `Decompose a task into minimal subtasks following MAKER principles.
Returns a list of atomic subtasks that can be executed independently.
Each subtask should be small enough that a correct solution is likely to be sampled.`,
        inputSchema: {
            type: 'object',
            properties: {
                task: {
                    type: 'string',
                    description: 'The task to decompose',
                },
                customSteps: {
                    type: 'array',
                    items: { type: 'string' },
                    description: 'Optional: provide custom decomposition steps instead of auto-generating',
                },
            },
            required: ['task'],
        },
    },
    {
        name: 'maker_check_red_flags',
        description: `Check a response for MAKER red flags that indicate unreliability.
Red flags include: excessive length (>750 tokens), hedging language, TODO/FIXME markers,
generic naming, and uncertainty markers. Returns list of detected issues.`,
        inputSchema: {
            type: 'object',
            properties: {
                response: {
                    type: 'string',
                    description: 'The LLM response to check for red flags',
                },
                maxTokens: {
                    type: 'number',
                    description: 'Optional: custom max token threshold (default: 750)',
                },
            },
            required: ['response'],
        },
    },
    {
        name: 'maker_submit_vote',
        description: `Submit a vote (response) for a subtask in an active MAKER session.
Implements "first-to-ahead-by-k" voting. Returns whether consensus was reached.
Use this to verify code by generating multiple solutions and voting on them.`,
        inputSchema: {
            type: 'object',
            properties: {
                subtaskId: {
                    type: 'string',
                    description: 'The subtask ID to vote on',
                },
                response: {
                    type: 'string',
                    description: 'The response/solution to submit as a vote',
                },
            },
            required: ['subtaskId', 'response'],
        },
    },
    {
        name: 'maker_get_session_status',
        description: `Get the current status of a MAKER task session.
Returns: current step, completed subtasks, voting progress, and statistics.`,
        inputSchema: {
            type: 'object',
            properties: {
                taskId: {
                    type: 'string',
                    description: 'The task ID to get status for',
                },
            },
            required: ['taskId'],
        },
    },
    {
        name: 'maker_calculate_k',
        description: `Calculate the optimal k (minimum votes) for a task.
Uses the formula from the MAKER paper: k_min = Θ(ln s).
Higher k means more reliability but higher cost.`,
        inputSchema: {
            type: 'object',
            properties: {
                totalSteps: {
                    type: 'number',
                    description: 'Total number of steps in the task',
                },
                successRate: {
                    type: 'number',
                    description: 'Estimated per-step success rate (0-1), default 0.95',
                },
                targetProbability: {
                    type: 'number',
                    description: 'Target overall success probability (0-1), default 0.95',
                },
            },
            required: ['totalSteps'],
        },
    },
    {
        name: 'maker_get_next_subtask',
        description: `Get the next pending subtask for a MAKER task.
Returns the subtask details and a formatted prompt for execution.`,
        inputSchema: {
            type: 'object',
            properties: {
                taskId: {
                    type: 'string',
                    description: 'The task ID',
                },
            },
            required: ['taskId'],
        },
    },
    {
        name: 'maker_list_tasks',
        description: `List all active MAKER task sessions.
Returns summary of each task including progress and status.`,
        inputSchema: {
            type: 'object',
            properties: {},
            required: [],
        },
    },
    {
        name: 'maker_configure',
        description: `Configure MAKER framework settings.
Adjust voting threshold, token limits, red-flagging behavior, etc.`,
        inputSchema: {
            type: 'object',
            properties: {
                minVotesForConsensus: {
                    type: 'number',
                    description: 'Minimum votes required for consensus (k)',
                },
                maxTokenThreshold: {
                    type: 'number',
                    description: 'Maximum tokens before flagging as too long',
                },
                enableRedFlagging: {
                    type: 'boolean',
                    description: 'Enable/disable red flag detection',
                },
            },
            required: [],
        },
    },
    {
        name: 'maker_validate_code',
        description: `Validate code using MAKER principles. Checks for red flags and provides
quality assessment. Use this before accepting any generated code.`,
        inputSchema: {
            type: 'object',
            properties: {
                code: {
                    type: 'string',
                    description: 'The code to validate',
                },
                language: {
                    type: 'string',
                    description: 'Programming language (for context-aware validation)',
                },
            },
            required: ['code'],
        },
    },
];
// ============================================================================
// Tool Handlers
// ============================================================================
server.setRequestHandler(ListToolsRequestSchema, async () => {
    return { tools: TOOLS };
});
server.setRequestHandler(CallToolRequestSchema, async (request) => {
    const { name, arguments: args } = request.params;
    switch (name) {
        case 'maker_analyze_task': {
            const { task } = args;
            const analysis = analyzeTask(task);
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            analysis: {
                                complexity: analysis.complexity,
                                estimatedSteps: analysis.estimatedSteps,
                                suggestedKMin: analysis.suggestedKMin,
                                estimatedCost: `~${analysis.estimatedCost.toFixed(0)} API calls`,
                                decompositionStrategy: analysis.decompositionStrategy,
                                warnings: analysis.warnings,
                            },
                            recommendation: analysis.complexity === 'very-high'
                                ? 'STRONGLY RECOMMENDED: Break this task down before proceeding'
                                : analysis.complexity === 'high'
                                    ? 'RECOMMENDED: Use maker_decompose_task to create subtasks'
                                    : 'Task is manageable, but decomposition will improve reliability',
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_decompose_task': {
            const { task, customSteps } = args;
            let steps;
            if (customSteps && customSteps.length > 0) {
                steps = customSteps;
            }
            else {
                steps = suggestDecomposition(task);
            }
            const decomposedTask = sessionManager.createTask(task, steps);
            const kCalc = sessionManager.calculateOptimalK(steps.length);
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            taskId: decomposedTask.id,
                            totalSubtasks: decomposedTask.subtasks.length,
                            subtasks: decomposedTask.subtasks.map(s => ({
                                id: s.id,
                                description: s.description,
                                status: s.status,
                            })),
                            votingConfig: {
                                kMin: kCalc.kMin,
                                expectedTotalVotes: decomposedTask.subtasks.length * kCalc.kMin,
                                targetSuccessProbability: `${(kCalc.successProbability * 100).toFixed(1)}%`,
                            },
                            nextStep: `Use maker_get_next_subtask with taskId "${decomposedTask.id}" to get the first subtask`,
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_check_red_flags': {
            const { response, maxTokens } = args;
            const config = {
                ...DEFAULT_CONFIG,
                maxTokenThreshold: maxTokens || DEFAULT_CONFIG.maxTokenThreshold
            };
            const redFlags = detectRedFlags(response, config);
            const estimatedTokens = Math.ceil(response.length / 4);
            const hasHighSeverity = redFlags.some(f => f.severity === 'high');
            const hasMediumSeverity = redFlags.some(f => f.severity === 'medium');
            let verdict;
            if (hasHighSeverity) {
                verdict = 'REJECT - High severity red flags detected. Regenerate with a more focused prompt.';
            }
            else if (hasMediumSeverity) {
                verdict = 'CAUTION - Medium severity issues detected. Consider regenerating or manual review.';
            }
            else if (redFlags.length > 0) {
                verdict = 'ACCEPTABLE - Minor issues detected. Safe to proceed with review.';
            }
            else {
                verdict = 'PASS - No red flags detected. Response appears reliable.';
            }
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            estimatedTokens,
                            maxTokenThreshold: config.maxTokenThreshold,
                            redFlagsFound: redFlags.length,
                            redFlags: redFlags.map(f => ({
                                type: f.type,
                                severity: f.severity,
                                description: f.description,
                            })),
                            verdict,
                            isValid: !hasHighSeverity,
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_submit_vote': {
            const { subtaskId, response } = args;
            const result = sessionManager.submitVote(subtaskId, response);
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            subtaskId,
                            votesReceived: result.session.responses.length,
                            leadingVoteCount: result.session.leadingVoteCount,
                            consensusReached: result.isComplete,
                            winningResponse: result.winner ? result.winner.substring(0, 200) + '...' : null,
                            redFlagsInThisVote: result.redFlags,
                            recommendation: result.isComplete
                                ? 'Consensus reached! Use maker_get_next_subtask for the next step.'
                                : 'Submit more votes to reach consensus.',
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_get_session_status': {
            const { taskId } = args;
            const task = sessionManager.getTask(taskId);
            if (!task) {
                return {
                    content: [
                        {
                            type: 'text',
                            text: JSON.stringify({ success: false, error: `Task ${taskId} not found` }),
                        },
                    ],
                };
            }
            const stats = sessionManager.getTaskStats(taskId);
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            taskId: task.id,
                            originalTask: task.originalTask,
                            status: task.status,
                            progress: {
                                currentStep: task.currentStep,
                                totalSteps: task.totalSteps,
                                percentComplete: ((task.currentStep / task.totalSteps) * 100).toFixed(1) + '%',
                            },
                            subtasks: task.subtasks.map(s => ({
                                id: s.id,
                                description: s.description.substring(0, 50) + '...',
                                status: s.status,
                                votesReceived: s.result?.votesReceived || 0,
                            })),
                            statistics: stats,
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_calculate_k': {
            const { totalSteps, successRate = 0.95, targetProbability = 0.95 } = args;
            const result = calculateKMin(totalSteps, successRate, targetProbability);
            const fullTaskProb = calculateFullTaskSuccessProbability(totalSteps, successRate, result.kMin);
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            inputs: {
                                totalSteps,
                                perStepSuccessRate: successRate,
                                targetProbability,
                            },
                            results: {
                                kMin: result.kMin,
                                description: `Each subtask needs ${result.kMin} consistent votes to pass`,
                                expectedTotalVotes: totalSteps * result.kMin,
                                predictedSuccessProbability: (fullTaskProb * 100).toFixed(2) + '%',
                                costScaling: `O(${totalSteps} × ln(${totalSteps})) = O(${Math.round(totalSteps * Math.log(totalSteps))})`,
                            },
                            interpretation: result.kMin <= 2
                                ? 'Low k indicates high confidence in per-step accuracy'
                                : result.kMin <= 4
                                    ? 'Moderate k - standard MAKER voting should work well'
                                    : 'High k suggests need for better decomposition or model selection',
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_get_next_subtask': {
            const { taskId } = args;
            const task = sessionManager.getTask(taskId);
            if (!task) {
                return {
                    content: [
                        {
                            type: 'text',
                            text: JSON.stringify({ success: false, error: `Task ${taskId} not found` }),
                        },
                    ],
                };
            }
            const nextSubtask = sessionManager.getNextSubtask(taskId);
            if (!nextSubtask) {
                return {
                    content: [
                        {
                            type: 'text',
                            text: JSON.stringify({
                                success: true,
                                message: 'All subtasks completed!',
                                taskStatus: task.status,
                                completedSubtasks: task.subtasks.filter(s => s.status === 'completed').length,
                            }),
                        },
                    ],
                };
            }
            const prompt = formatSubtaskPrompt(nextSubtask);
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            subtask: {
                                id: nextSubtask.id,
                                description: nextSubtask.description,
                                stepNumber: task.currentStep + 1,
                                totalSteps: task.totalSteps,
                            },
                            prompt,
                            instructions: [
                                '1. Execute this subtask to generate a solution',
                                '2. Use maker_check_red_flags to validate the response',
                                '3. Use maker_submit_vote to submit the response',
                                '4. Repeat steps 1-3 until consensus is reached',
                                '5. Call maker_get_next_subtask for the next step',
                            ],
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_list_tasks': {
            const tasks = sessionManager.getAllTasks();
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            totalTasks: tasks.length,
                            tasks: tasks.map(t => ({
                                id: t.id,
                                originalTask: t.originalTask.substring(0, 100) + '...',
                                status: t.status,
                                progress: `${t.currentStep}/${t.totalSteps}`,
                                createdAt: t.createdAt.toISOString(),
                            })),
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_configure': {
            const updates = args;
            const newConfig = sessionManager.updateConfig(updates);
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            message: 'Configuration updated',
                            currentConfig: {
                                minVotesForConsensus: newConfig.minVotesForConsensus,
                                maxTokenThreshold: newConfig.maxTokenThreshold,
                                targetSuccessProbability: newConfig.targetSuccessProbability,
                                enableRedFlagging: newConfig.enableRedFlagging,
                                maxAttemptsPerSubtask: newConfig.maxAttemptsPerSubtask,
                            },
                        }, null, 2),
                    },
                ],
            };
        }
        case 'maker_validate_code': {
            const { code, language } = args;
            const redFlags = detectRedFlags(code, DEFAULT_CONFIG);
            const lines = code.split('\n').length;
            const estimatedTokens = Math.ceil(code.length / 4);
            // Additional code-specific checks
            const codeIssues = [];
            if (lines > 50) {
                codeIssues.push(`Code is ${lines} lines. Consider breaking into smaller functions.`);
            }
            if (/console\.(log|debug|info)/g.test(code)) {
                codeIssues.push('Contains console.log statements - may need cleanup');
            }
            if (/any\s*[;,)>]/g.test(code) && language?.toLowerCase().includes('typescript')) {
                codeIssues.push('Contains "any" type - consider adding proper types');
            }
            if (!/\b(test|spec|describe|it|expect)\b/i.test(code) && lines > 30) {
                codeIssues.push('No tests detected for substantial code block');
            }
            const hasHighSeverity = redFlags.some(f => f.severity === 'high');
            const score = 100 - (redFlags.length * 10) - (codeIssues.length * 5);
            return {
                content: [
                    {
                        type: 'text',
                        text: JSON.stringify({
                            success: true,
                            metrics: {
                                lines,
                                estimatedTokens,
                                isWithinMAKERGuidelines: lines <= 50 && estimatedTokens <= 750,
                            },
                            redFlags: redFlags.map(f => ({
                                type: f.type,
                                severity: f.severity,
                                description: f.description,
                            })),
                            codeIssues,
                            qualityScore: Math.max(0, score),
                            verdict: hasHighSeverity
                                ? 'REJECT - Code has critical issues'
                                : score >= 80
                                    ? 'ACCEPT - Code meets MAKER quality standards'
                                    : score >= 60
                                        ? 'REVIEW - Code needs minor improvements'
                                        : 'REVISE - Code has significant issues',
                            recommendations: [
                                lines > 50 ? 'Break into smaller, focused functions' : null,
                                estimatedTokens > 750 ? 'Simplify implementation to reduce complexity' : null,
                                hasHighSeverity ? 'Address high-severity red flags before proceeding' : null,
                            ].filter(Boolean),
                        }, null, 2),
                    },
                ],
            };
        }
        default:
            throw new Error(`Unknown tool: ${name}`);
    }
});
// ============================================================================
// Prompt Templates
// ============================================================================
const PROMPTS = [
    {
        name: 'maker_workflow',
        description: 'Complete MAKER workflow for a coding task',
        arguments: [
            {
                name: 'task',
                description: 'The coding task to complete using MAKER methodology',
                required: true,
            },
        ],
    },
    {
        name: 'maker_quick_check',
        description: 'Quick MAKER validation for generated code',
        arguments: [
            {
                name: 'code',
                description: 'The code to validate',
                required: true,
            },
        ],
    },
];
server.setRequestHandler(ListPromptsRequestSchema, async () => {
    return { prompts: PROMPTS };
});
server.setRequestHandler(GetPromptRequestSchema, async (request) => {
    const { name, arguments: args } = request.params;
    switch (name) {
        case 'maker_workflow':
            return {
                messages: [
                    {
                        role: 'user',
                        content: {
                            type: 'text',
                            text: `# MAKER Workflow for Task

## Task Description
${args?.task || '[No task provided]'}

## Instructions

Follow the MAKER (Massively Decomposed Agentic Process) methodology:

### Step 1: Analyze
First, use \`maker_analyze_task\` to understand the complexity and get recommendations.

### Step 2: Decompose
Use \`maker_decompose_task\` to break the task into minimal subtasks. Each subtask should:
- Have a single, clear responsibility
- Be implementable in <50 lines of code
- Have verifiable output

### Step 3: Execute with Voting
For each subtask:
1. Generate a solution
2. Use \`maker_check_red_flags\` to validate
3. If valid, use \`maker_submit_vote\` 
4. Repeat until consensus (k votes agree)

### Step 4: Validate
Use \`maker_validate_code\` on the final combined solution.

### Key Principles
- **Decomposition**: Smaller is better. If unsure, break it down more.
- **Red-flagging**: Discard responses with TODO, FIXME, hedging language, or >750 tokens
- **Voting**: Multiple consistent responses = higher confidence
- **Focus**: Each subtask gets minimal context for its specific job

Begin by analyzing the task.`,
                        },
                    },
                ],
            };
        case 'maker_quick_check':
            return {
                messages: [
                    {
                        role: 'user',
                        content: {
                            type: 'text',
                            text: `# MAKER Quick Validation

Validate this code using MAKER principles:

\`\`\`
${args?.code || '[No code provided]'}
\`\`\`

Use \`maker_validate_code\` and \`maker_check_red_flags\` to assess quality.

Report:
1. Red flags found
2. Quality score
3. Whether to ACCEPT, REVIEW, or REJECT
4. Specific recommendations for improvement`,
                        },
                    },
                ],
            };
        default:
            throw new Error(`Unknown prompt: ${name}`);
    }
});
// ============================================================================
// Server Startup
// ============================================================================
async function main() {
    const transport = new StdioServerTransport();
    await server.connect(transport);
    console.error('MAKER MCP Server running on stdio');
}
main().catch((error) => {
    console.error('Fatal error:', error);
    process.exit(1);
});
//# sourceMappingURL=index.js.map
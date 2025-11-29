# MAKER MCP Server

An MCP (Model Context Protocol) server implementing the **MAKER framework** for massively decomposed agentic processes.

Based on the paper: **"Solving a Million-Step LLM Task with Zero Errors"** (arXiv:2511.09030v1, November 2025)

## Overview

MAKER achieves near-zero error rates in LLM tasks through three core mechanisms:

1. **Extreme Decomposition** - Breaking tasks into minimal subtasks
2. **Voting-based Error Correction** - Multiple responses with consensus
3. **Red-Flagging** - Discarding unreliable responses

This MCP server brings these capabilities to VS Code GitHub Copilot.

## Installation

### 1. Install Dependencies

```powershell
cd "e:\Futures Arbitrage\MAKER\mcp-maker-server"
npm install
```

### 2. Build the Server

```powershell
npm run build
```

### 3. Configure VS Code

Add to your VS Code settings (`settings.json`) or use the workspace `.vscode/mcp.json`:

```json
{
  "mcp": {
    "servers": {
      "maker": {
        "command": "node",
        "args": ["e:/Futures Arbitrage/MAKER/mcp-maker-server/dist/index.js"]
      }
    }
  }
}
```

Or for User-level settings, add to `%APPDATA%\Code\User\settings.json`:

```json
{
  "github.copilot.chat.mcpServers": {
    "maker": {
      "command": "node",
      "args": ["e:/Futures Arbitrage/MAKER/mcp-maker-server/dist/index.js"]
    }
  }
}
```

### 4. Restart VS Code

Restart VS Code to load the MCP server.

## Available Tools

### `maker_analyze_task`
Analyze a task to determine complexity and get MAKER recommendations.

```
Use: Before starting any complex coding task
Returns: complexity, estimated steps, suggested k_min, decomposition strategy
```

### `maker_decompose_task`
Decompose a task into minimal subtasks following MAKER principles.

```
Use: After analyzing, to create a task session
Returns: Task ID, list of subtasks, voting configuration
```

### `maker_check_red_flags`
Check a response for MAKER red flags indicating unreliability.

**Red flags detected:**
- Excessive length (>750 tokens)
- Hedging language ("maybe", "perhaps", "might")
- TODO/FIXME markers
- Generic naming (data, result, temp)
- Uncertainty markers (..., ???)

### `maker_submit_vote`
Submit a vote for a subtask. Implements "first-to-ahead-by-k" voting.

```
Use: After generating a solution for a subtask
Returns: Whether consensus was reached, leading vote count
```

### `maker_validate_code`
Validate code using MAKER principles with quality scoring.

```
Returns: Quality score (0-100), verdict (ACCEPT/REVIEW/REJECT), recommendations
```

### `maker_calculate_k`
Calculate optimal k (minimum votes) for a task using the formula:
```
k_min = Θ(ln s)
```

### `maker_get_next_subtask`
Get the next pending subtask for a MAKER task.

### `maker_get_session_status`
Get current status of a MAKER task session.

### `maker_list_tasks`
List all active MAKER task sessions.

### `maker_configure`
Adjust MAKER settings (voting threshold, token limits, etc.)

## Usage Examples

### Example 1: Validate Generated Code

```
@maker Please check this code for red flags:

function processData(data) {
  // TODO: implement error handling
  const result = data.map(x => x * 2);
  return result;
}
```

Copilot will use `maker_check_red_flags` and report issues.

### Example 2: Complete MAKER Workflow

```
@maker I need to implement a WebSocket connection manager with:
- Connection pooling
- Automatic reconnection
- Message queuing
- Health monitoring

Please use the full MAKER workflow.
```

Copilot will:
1. Analyze the task
2. Decompose into subtasks
3. Execute each with voting
4. Validate the final result

### Example 3: Quick Quality Check

```
@maker Validate this implementation and give me a quality score:

[paste your code here]
```

## MAKER Principles for Better Code

### 1. Decomposition Guidelines

| Task Size | Action |
|-----------|--------|
| >50 lines | Break into smaller functions |
| >3 responsibilities | Split into separate modules |
| Complex logic | Extract into pure functions |

### 2. Red Flag Thresholds

| Indicator | Threshold | Action |
|-----------|-----------|--------|
| Response length | >750 tokens | Reject and regenerate |
| TODO/FIXME | Any | Reject |
| Hedging language | >2 instances | Review carefully |
| Generic names | >3 variables | Reject |

### 3. Voting Strategy

- **k=2**: Quick validation for simple tasks
- **k=3**: Standard for most coding tasks (default)
- **k=5**: Critical code paths, security-sensitive code

## Configuration

Default configuration (adjustable via `maker_configure`):

```json
{
  "minVotesForConsensus": 3,
  "maxTokenThreshold": 750,
  "targetSuccessProbability": 0.95,
  "maxAttemptsPerSubtask": 10,
  "enableRedFlagging": true
}
```

## Theoretical Background

### Key Formulas from the Paper

**Per-step success probability after voting:**
```
p(correct) = p^k / (p^k + (1-p)^k)
```

**Minimum k for target probability:**
```
k_min = ⌈ln(t^(-m/s) - 1) / ln((1-p)/p)⌉ = Θ(ln s)
```

**Expected cost (maximal decomposition):**
```
E[cost] = Θ(s ln s)
```

This log-linear scaling is what makes MAKER practical for tasks with thousands or millions of steps.

## Troubleshooting

### Server Not Loading

1. Ensure the server is built: `npm run build`
2. Check the path in your MCP configuration
3. Restart VS Code completely
4. Check Developer Tools (Help > Toggle Developer Tools) for errors

### Red Flags Too Aggressive

Adjust the configuration:
```
@maker Configure MAKER with maxTokenThreshold: 1000 and enableRedFlagging: false
```

### Voting Takes Too Long

Lower the k value for simpler tasks:
```
@maker Configure MAKER with minVotesForConsensus: 2
```

## Development

### Run in Development Mode

```powershell
npm run dev
```

### Watch for Changes

```powershell
npm run watch
```

## License

MIT

## References

- Paper: "Solving a Million-Step LLM Task with Zero Errors" (arXiv:2511.09030v1)
- Authors: Elliot Meyerson et al., Cognizant AI Lab & UT Austin
- MCP SDK: https://github.com/modelcontextprotocol/sdk

#!/bin/bash
# Creates all RAFT-KV project tasks as GitHub issues and adds them to the project board
# Usage: ./create_tasks.sh
#
# Prerequisites:
#   brew install gh
#   gh auth login
#   Update OWNER, REPO, PROJECT_NUMBER below

OWNER="zhengyicoding"
REPO="raft-kv"
PROJECT_NUMBER=1  # update this if different — check your project URL

# Helper: create issue and add to project
create_task() {
  local title="$1"
  local body="$2"
  local label="$3"

  echo "Creating: $title"
  issue_url=$(gh issue create \
    --repo "$OWNER/$REPO" \
    --title "$title" \
    --body "$body" \
    --label "$label" \
    --assignee "$OWNER" 2>/dev/null | tail -1)

  gh project item-add "$PROJECT_NUMBER" \
    --owner "$OWNER" \
    --url "$issue_url" 2>/dev/null

  echo "  ✅ Added: $title"
}

# Create labels first (ignore errors if they already exist)
gh label create "week-1" --color "0075ca" --repo "$OWNER/$REPO" 2>/dev/null
gh label create "week-2" --color "e4e669" --repo "$OWNER/$REPO" 2>/dev/null
gh label create "week-3" --color "d93f0b" --repo "$OWNER/$REPO" 2>/dev/null
gh label create "week-4" --color "0e8a16" --repo "$OWNER/$REPO" 2>/dev/null

echo ""
echo "=== Week 1 (Done) ==="
create_task "Implement Raft FSM (PUT/DELETE/GET, BoltDB, snapshot)" "Owner: All | Due: Mar 30" "week-1"
create_task "Implement HTTP API (write forwarding, consistency modes)" "Owner: All | Due: Mar 30" "week-1"
create_task "Implement Store API (Barrier, staleness check)" "Owner: All | Due: Mar 30" "week-1"
create_task "Local 3-node cluster setup and functional testing" "Owner: All | Due: Mar 30" "week-1"
create_task "Python correctness checker" "Owner: Zhengyi | Due: Mar 30" "week-1"
create_task "Env var refactor (separate process per node)" "Owner: Zhengyi | Due: Mar 30" "week-1"
create_task "Push codebase to GitHub" "Owner: Zhengyi | Due: Mar 30" "week-1"

echo ""
echo "=== Week 2 (In Progress) ==="
create_task "Review individual implementations, agree on combined codebase" "Owner: All | Due: Apr 1" "week-2"
create_task "Terraform: provision 5 EC2 t3.micro instances" "Owner: Siwen | Due: Apr 3" "week-2"
create_task "Security groups and networking for EC2 cluster" "Owner: Siwen | Due: Apr 3" "week-2"
create_task "Deploy binary to EC2, verify end-to-end 5-node cluster" "Owner: Siwen | Due: Apr 5" "week-2"
create_task "Locust load generator (writes/sec, read/write mix)" "Owner: Wenyu | Due: Apr 5" "week-2"
create_task "Chaos scripts: leader kill and restart automation" "Owner: Qian | Due: Apr 5" "week-2"
create_task "Chaos scripts: iptables partition injection" "Owner: Zhengyi | Due: Apr 5" "week-2"
create_task "Structured JSON logging on each node" "Owner: Siwen | Due: Apr 5" "week-2"
create_task "S3 log collection pipeline" "Owner: Siwen | Due: Apr 6" "week-2"
create_task "Milestone 1 report and video submission" "Owner: All | Due: Mar 31" "week-2"

echo ""
echo "=== Week 3 (Backlog) ==="
create_task "Experiment 1: election timeout sweep (150/300/500ms)" "Owner: Qian | Due: Apr 10" "week-3"
create_task "Experiment 1: back-to-back leader kill test" "Owner: Qian | Due: Apr 10" "week-3"
create_task "Experiment 2: minority partition (3 vs 2 nodes)" "Owner: Zhengyi | Due: Apr 10" "week-3"
create_task "Experiment 2: leader isolation partition" "Owner: Zhengyi | Due: Apr 10" "week-3"
create_task "Experiment 2: symmetric 2-2-1 split" "Owner: Zhengyi | Due: Apr 10" "week-3"
create_task "Experiment 2: post-healing correctness check" "Owner: Zhengyi | Due: Apr 10" "week-3"
create_task "Experiment 3: read scaling (3/4/5 nodes)" "Owner: Wenyu | Due: Apr 10" "week-3"
create_task "Experiment 3: consistency mode benchmark (strong/default/stale)" "Owner: Wenyu | Due: Apr 10" "week-3"
create_task "Collect all experiment logs to S3" "Owner: Siwen | Due: Apr 11" "week-3"
create_task "Generate matplotlib charts for all experiments" "Owner: Siwen | Due: Apr 12" "week-3"

echo ""
echo "=== Week 4 (Backlog) ==="
create_task "Analyze Experiment 1 results" "Owner: Qian | Due: Apr 15" "week-4"
create_task "Analyze Experiment 2 results" "Owner: Zhengyi | Due: Apr 15" "week-4"
create_task "Analyze Experiment 3 results" "Owner: Wenyu | Due: Apr 15" "week-4"
create_task "Write final report" "Owner: All | Due: Apr 18" "week-4"
create_task "Prepare presentation slides" "Owner: All | Due: Apr 19" "week-4"
create_task "Final submission" "Owner: All | Due: Apr 20" "week-4"

echo ""
echo "🎉 All tasks created! Visit: https://github.com/users/$OWNER/projects/$PROJECT_NUMBER"
package basecamp

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// Agents are the people of the account a run writes as agents, each with the
// human they act for, both by their Basecamp ID. Basecamp has no agents: an
// agent there is a user like any other, and only whoever runs the import knows
// which of them are one.
//
// Temporary: it serves the moving over of one account, and goes once that is
// done, with this file, the flag that fills it, bundle.Agent and the calls
// marked as the agents'.
type Agents map[int64]int64

// agentPair is what --agent takes: two Basecamp IDs as Basecamp writes them,
// without a sign or a leading zero, so that each is the sourceId of a person.
var agentPair = regexp.MustCompile(`^([1-9][0-9]{0,17})=([1-9][0-9]{0,17})$`)

// ParseAgents reads the values of --agent. It refuses what no account could
// make right: a value of another shape, an agent of itself, an agent named for
// two humans, and somebody named as an agent and as the human of one, since
// an agent acts for a human and never for another agent.
func ParseAgents(values []string) (Agents, error) {
	agents := Agents{}
	for _, value := range values {
		pair := agentPair.FindStringSubmatch(value)
		if pair == nil {
			return nil, agentsError(fmt.Sprintf("--agent %q is not <agent ID>=<human ID>; Basecamp IDs are numbers such as 12345678", value))
		}
		agent, _ := strconv.ParseInt(pair[1], 10, 64)
		human, _ := strconv.ParseInt(pair[2], 10, 64)
		if agent == human {
			return nil, agentsError(fmt.Sprintf("--agent names person %d as their own agent", agent))
		}
		if named, twice := agents[agent]; twice && named != human {
			return nil, agentsError(fmt.Sprintf("--agent names person %d as the agent of %d and of %d; an agent acts for one human", agent, named, human))
		}
		agents[agent] = human
	}
	for _, agent := range agents.sorted() {
		if _, isAnAgent := agents[agents[agent]]; isAnAgent {
			return nil, agentsError(fmt.Sprintf("--agent names person %d as the human of %d and as an agent; an agent acts for a human", agents[agent], agent))
		}
	}
	return agents, nil
}

// sorted are the agents in the order of their IDs, so that a refusal names the
// same one on every run.
func (a Agents) sorted() []int64 {
	ids := make([]int64, 0, len(a))
	for agent := range a {
		ids = append(ids, agent)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// takeAgents holds the agents against the list of the account, before any
// project is asked for. Agent and human alike must be users of the account
// who are no clients: a chatbot, somebody deleted and a client act for nobody
// and nobody acts for them, and somebody the list does not hold cannot be told
// to be any of these. Their times are checked here as well, since a human may
// come in only where the people are written, after a walk of hours.
func (r *registry) takeAgents(agents Agents) error {
	for _, agent := range agents.sorted() {
		if err := r.mayBeOfAnAgent("an agent", agent); err != nil {
			return err
		}
		if err := r.mayBeOfAnAgent(fmt.Sprintf("the human of %d", agent), agents[agent]); err != nil {
			return err
		}
	}
	r.agents = agents
	return nil
}

func (r *registry) mayBeOfAnAgent(as string, id int64) error {
	p, listed := r.account[id]
	switch {
	case !listed:
		return agentsError(fmt.Sprintf("--agent names person %d as %s, who is not in the people list of the account", id, as))
	case p.PersonableType != "User":
		return agentsError(fmt.Sprintf("--agent names person %d as %s, who is no user of the account", id, as))
	case p.Client:
		return agentsError(fmt.Sprintf("--agent names person %d as %s, who is a client", id, as))
	}
	return requireTimestamps(fmt.Sprintf("person %d", id), p.CreatedAt, p.UpdatedAt)
}

// isAnAgent tells that the run writes somebody as an agent.
func (r *registry) isAnAgent(id int64) bool {
	_, isAnAgent := r.agents[id]
	return isAnAgent
}

// principalOf is the sourceId of the human somebody is the agent of.
func (r *registry) principalOf(id int64) (string, bool) {
	human, isAnAgent := r.agents[id]
	return sourceID(human), isAnAgent
}

// bringInThePrincipals brings the human of every agent of the bundle in, since
// the agent refers to them: a run whose records name the agent alone writes
// the human all the same. An agent no record names stays out like anybody
// else, and so does their human, if nothing else names them.
func (r *registry) bringInThePrincipals() error {
	for _, agent := range r.agents.sorted() {
		if _, referred := r.referred[agent]; !referred {
			continue
		}
		if _, _, err := r.referByID(r.agents[agent]); err != nil {
			return err
		}
	}
	return nil
}

// inWritingOrder are the people of the bundle in the order of their IDs, the
// agents last, so that the human an agent names stands before the agent.
func (r *registry) inWritingOrder() []seenPerson {
	people := r.sorted()
	sort.SliceStable(people, func(i, j int) bool {
		_, iIsAnAgent := r.agents[people[i].ID]
		_, jIsAnAgent := r.agents[people[j].ID]
		return !iIsAnAgent && jIsAnAgent
	})
	return people
}

// agentsError is a refusal of what --agent names.
func agentsError(message string) error {
	return &Error{Kind: Usage, Message: message,
		Hint: "--agent takes <agent ID>=<human ID>, both people of the Basecamp account who are users and no clients, an agent named for one human only and never as the human of another. Look the IDs up with: basecamp people list"}
}

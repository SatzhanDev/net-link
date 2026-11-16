package server

import (
	"strings"

	"net-link/internal/chatprofile"
)

// This file contains the chat room logic: who is in the room, the message
// history and how messages are delivered. Network handling lives in
// server.go and session.go.
//
// Every function here runs under s.mu, so the order of events is the same
// for every client and in the chat log file.

// join adds p to the chat atomically: it checks that the name is free,
// sends p the history and tells everybody else that p has joined.
// Doing all of it under one lock guarantees that p sees every message
// exactly once: either in the history or as a live broadcast.
func (s *Server) join(p *participant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrServerClosed
	}
	if s.nameTakenLocked(p.name, nil) {
		return ErrNameTaken
	}
	s.members[p] = struct{}{}

	now := s.now()
	var b strings.Builder
	for _, line := range s.history {
		b.WriteString(line)
		b.WriteString("\n")
	}
	// Right after the history comes the live session profile.
	b.WriteString(formatSystemNote(s.profile.Summary(now)))
	b.WriteString("\n")
	b.WriteString(prompt(now, p.name))
	// One enqueue for the whole history, so a long history cannot
	// overflow the outbox.
	p.enqueue(b.String())

	notice := formatSystem(now, p.name+" has joined our chat.")
	s.logLocked(notice)
	s.broadcastLocked(notice, p)
	return nil
}

// leave removes p from the chat and notifies the others.
func (s *Server) leave(p *participant) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.members[p]; !ok {
		return
	}
	delete(s.members, p)
	notice := formatSystem(s.now(), p.name+" has left our chat.")
	s.logLocked(notice)
	s.broadcastLocked(notice, p)
}

// publish stores a chat message and sends it to everyone except the author.
// The author only gets a fresh prompt: their own text is already on screen.
// A client that floods the chat also gets a private warning; the message
// itself is still delivered (the subject only asks for a warning).
func (s *Server) publish(p *participant, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	msg := formatMessage(now, p.name, text)
	s.history = append(s.history, msg)
	s.profile.Record(p.name)
	s.logLocked(msg)
	s.broadcastLocked(msg, p)

	p.recent = chatprofile.RecentMessages(p.recent, now, chatprofile.FloodWindow)
	if chatprofile.IsFlooding(len(p.recent), chatprofile.FloodLimit) {
		s.logLocked(formatSystem(now, "flood warning sent to "+p.name+"."))
		p.enqueue(formatSystemNote(chatprofile.FloodWarning) + "\n")
	}
	p.enqueue(prompt(now, p.name))
}

// rename changes p's name and informs the whole group, including p, so the
// user sees that the change worked. The message counters of the profiler
// move to the new name. Changing only the letter case of your own name
// ("lee" -> "Lee") is allowed.
func (s *Server) rename(p *participant, newName string) error {
	if err := validateName(newName); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if newName == p.name {
		return ErrNameSame
	}
	if s.nameTakenLocked(newName, p) {
		return ErrNameTaken
	}

	oldName := p.name
	// p.name is written only here, under s.mu, by p's own goroutine.
	// Other goroutines read it only under s.mu, so there is no data race.
	p.name = newName
	s.profile.Rename(oldName, newName)

	now := s.now()
	notice := formatSystem(now, oldName+" has changed their name to "+newName+".")
	s.logLocked(notice)
	s.broadcastLocked(notice, p)
	p.enqueue(notice + "\n" + prompt(now, p.name))
	return nil
}

// nameTakenLocked reports whether another member (not `self`) already uses
// name, ignoring letter case. Caller must hold s.mu.
func (s *Server) nameTakenLocked(name string, self *participant) bool {
	for m := range s.members {
		if m != self && strings.EqualFold(m.name, name) {
			return true
		}
	}
	return false
}

// broadcastLocked delivers msg to every member except `except`.
// Each recipient's half-typed prompt line is replaced by the message,
// then a new prompt is drawn. Caller must hold s.mu.
func (s *Server) broadcastLocked(msg string, except *participant) {
	now := s.now()
	for m := range s.members {
		if m == except {
			continue
		}
		m.enqueue(clearLine + msg + "\n" + prompt(now, m.name))
	}
}

// logLocked appends a line to the chat log file, if one is configured.
// Caller must hold s.mu, which also keeps lines from interleaving.
func (s *Server) logLocked(line string) {
	if s.chatLog != nil {
		s.chatLog.Println(line)
	}
}

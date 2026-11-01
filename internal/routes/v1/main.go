package v1

import (
	"github.com/labstack/echo/v4"

	"travel-service/internal/auth"
	handlers "travel-service/internal/handler"
)

// Load mounts all v1 routes. Auth (register/login) is public; everything else
// sits behind the JWT middleware. Trip-scoped permission checks happen in the
// service layer, not here (core rule 7).
func Load(router *echo.Group) {
	ah := handlers.NewAuthHandler()
	ph := handlers.NewParticipantHandler()
	th := handlers.NewTripHandler()
	tl := handlers.NewTimelineHandler()
	tr := handlers.NewTransportHandler()
	eh := handlers.NewExpenseHandler()
	sh := handlers.NewSettlementHandler()
	smh := handlers.NewSummaryHandler()
	pub := handlers.NewPublicHandler()
	ev := handlers.NewEventHandler()
	rsvp := handlers.NewRSVPHandler()
	ppl := handlers.NewPeopleHandler()
	poolH := handlers.NewPoolHandler()
	cmt := handlers.NewCommentHandler()
	nh := handlers.NewNotificationHandler()

	// Public — no auth (a bearer token, if sent, is read only where noted optional)
	router.POST("/auth/register", ah.Register)
	router.POST("/auth/login", ah.Login)
	router.GET("/public/trips", pub.List)
	router.GET("/public/trips/:id", pub.Get)
	router.GET("/join/:token", rsvp.Join)              // auth optional
	router.GET("/users/:id/profile", ppl.Profile)      // auth optional
	router.GET("/public/trips/:id/comments", cmt.List) // auth optional

	// Authenticated
	p := router.Group("", auth.Middleware)
	p.PUT("/join/:token/rsvp", rsvp.Reply)
	p.POST("/public/trips/:id/comments", cmt.Create)
	p.PUT("/comments/:id", cmt.Update)
	p.DELETE("/comments/:id", cmt.Delete)

	// People graph
	p.POST("/users/:id/follow", ppl.Follow)
	p.DELETE("/users/:id/follow", ppl.Unfollow)
	p.POST("/users/:id/friend", ppl.Friend)
	p.DELETE("/users/:id/friend", ppl.Unfriend)
	p.GET("/me/friends", ppl.Friends)
	p.POST("/users/:id/block", ppl.Block)
	p.DELETE("/users/:id/block", ppl.Unblock)
	p.GET("/me/blocked", ppl.Blocked)
	p.POST("/reports", ppl.Report)
	p.GET("/me/notifications", nh.List)
	p.POST("/me/notifications/read", nh.MarkRead)
	p.GET("/me", ah.Me)
	p.PUT("/me", ah.UpdateProfile)
	p.POST("/invites/accept", ph.Accept)
	p.GET("/me/saved-trips", pub.ListSaved)
	p.POST("/public/trips/:id/copy", pub.Copy) // copy-as-template needs the caller
	p.PUT("/public/trips/:id/save", pub.Save)
	p.DELETE("/public/trips/:id/save", pub.Unsave)
	p.POST("/public/trips/:id/like", pub.Like)
	p.DELETE("/public/trips/:id/like", pub.Unlike)

	t := p.Group("/trips")
	t.POST("", th.Create)
	t.GET("", th.List)
	t.GET("/:id", th.Get)
	t.PUT("/:id", th.Update)
	t.DELETE("/:id", th.Delete)
	t.POST("/:id/publish", th.SetPublic)
	t.POST("/:id/archive", th.Archive)
	t.PUT("/:id/budget", th.SetBudget)
	t.GET("/:id/activity", ev.Activity)
	t.POST("/:id/destinations", th.AddDestination)
	t.DELETE("/:id/destinations/:destID", th.DeleteDestination)

	// RSVP invite links (organiser = trip owner)
	t.GET("/:id/invite-link", rsvp.GetLink)
	t.PUT("/:id/invite-link", rsvp.UpsertLink)
	t.POST("/:id/invite-link/reset", rsvp.ResetLink)
	t.GET("/:id/rsvps", rsvp.ListRSVPs)
	t.POST("/:id/rsvps/:rsvpId/approve", rsvp.Approve)
	t.DELETE("/:id/rsvps/:rsvpId", rsvp.DeleteRSVP)

	// Participants & invitations
	t.POST("/:id/participants", ph.Invite)
	t.GET("/:id/participants", ph.List)
	t.PUT("/:id/participants/:pid/role", ph.ChangeRole)
	t.DELETE("/:id/participants/:pid", ph.Remove)

	// Timeline
	t.POST("/:id/entries", tl.CreateEntry)
	t.GET("/:id/entries", tl.ListEntries)
	t.PUT("/:id/entries/:eid", tl.UpdateEntry)
	t.DELETE("/:id/entries/:eid", tl.DeleteEntry)

	// Photos
	t.POST("/:id/photos", tl.AddPhoto)
	t.GET("/:id/photos", tl.ListPhotos)
	t.DELETE("/:id/photos/:pid", tl.DeletePhoto)

	// Transportation
	t.POST("/:id/transportation", tr.Create)
	t.GET("/:id/transportation", tr.List)
	t.PUT("/:id/transportation/:tid", tr.Update)
	t.DELETE("/:id/transportation/:tid", tr.Delete)

	// Expenses & external payers
	t.POST("/:id/expenses", eh.Create)
	t.GET("/:id/expenses", eh.List)
	t.GET("/:id/expenses/:eid", eh.Get)
	t.PUT("/:id/expenses/:eid", eh.Update)
	t.DELETE("/:id/expenses/:eid", eh.Delete)
	t.GET("/:id/external-payers", eh.ListExternalPayers)
	t.POST("/:id/external-payers", eh.CreateExternalPayer)

	// Settlements & balances
	t.GET("/:id/balances", sh.Balances)
	t.GET("/:id/settlements", sh.List)
	t.POST("/:id/settlements", sh.Create)
	t.POST("/:id/settlements/:sid/confirm", sh.Confirm)
	t.DELETE("/:id/settlements/:sid", sh.Delete)

	// Trip pool (optional)
	t.GET("/:id/pool", poolH.Get)
	t.PUT("/:id/pool", poolH.Set)
	t.DELETE("/:id/pool", poolH.Delete)

	// Summary
	t.GET("/:id/summary", smh.Get)
}

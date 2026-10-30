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

	// Public — no auth (a bearer token, if sent, is ignored)
	router.POST("/auth/register", ah.Register)
	router.POST("/auth/login", ah.Login)
	router.GET("/public/trips", pub.List)
	router.GET("/public/trips/:id", pub.Get)

	// Authenticated
	p := router.Group("", auth.Middleware)
	p.GET("/me", ah.Me)
	p.PUT("/me", ah.UpdateProfile)
	p.POST("/invites/accept", ph.Accept)
	p.GET("/me/saved-trips", pub.ListSaved)
	p.POST("/public/trips/:id/copy", pub.Copy) // copy-as-template needs the caller
	p.PUT("/public/trips/:id/save", pub.Save)
	p.DELETE("/public/trips/:id/save", pub.Unsave)

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
	t.DELETE("/:id/settlements/:sid", sh.Delete)

	// Summary
	t.GET("/:id/summary", smh.Get)
}

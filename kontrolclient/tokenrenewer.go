package kontrolclient

// // Parse token for setting validUntil field
//         if auth.Type == "token" && auth.validUntil == nil {
//                 var exp time.Time
//                 token, err := jwt.Parse(auth.Key, k.getRSAKey)
//                 if err != nil {
//                         exp = time.Now().UTC()
//                 } else {
//                         exp = time.Unix(int64(token.Claims["exp"].(float64)), 0).UTC()
//                 }
//                 r.Authentication.validUntil = &exp
//         }

//         r.OnConnect(func() {
//                 if r.Authentication.Type != "token" {
//                         return
//                 }

//                 // Start a goroutine that will renew the token before it expires.
//                 r.startTokenRenewer()
//         })

// func (r *RemoteKite) renewToken() error {
//         tokenString, err := r.localKite.Kontrol.GetToken(&r.Kite)
//         if err != nil {
//                 return err
//         }

//         token, err := jwt.Parse(tokenString, r.localKite.getRSAKey)
//         if err != nil {
//                 return fmt.Errorf("Cannot parse token: %s", err.Error())
//         }

//         exp := time.Unix(int64(token.Claims["exp"].(float64)), 0).UTC()

//         r.Authentication.Key = tokenString
//         r.Authentication.validUntil = &exp

//         return nil
// }

// func (r *RemoteKite) startTokenRenewer() {
//         const (
//                 renewBefore   = 30 * time.Second
//                 retryInterval = 10 * time.Second
//         )

//         // The duration from now to the time token needs to be renewed.
//         // Needs to be calculated after renewing the token.
//         renewDuration := func() time.Duration {
//                 return r.Authentication.validUntil.Add(-renewBefore).Sub(time.Now().UTC())
//         }

//         // renews token before it expires (sends the first signal to the goroutine below)
//         go time.AfterFunc(renewDuration(), r.sendRenewTokenSignal)

//         // renews token on signal
//         go func() {
//                 for {
//                         select {
//                         case <-r.signalRenewToken:
//                                 if err := r.renewToken(); err != nil {
//                                         r.Log.Error("token renewer: %s Cannot renew token for Kite: %s I will retry in %d seconds...", err.Error(), r.Kite.ID, retryInterval/time.Second)
//                                         // Need to sleep here litle bit because a signal is sent
//                                         // when an expired token is detected on incoming request.
//                                         // This sleep prevents the signal from coming too fast.
//                                         time.Sleep(1 * time.Second)
//                                         go time.AfterFunc(retryInterval, r.sendRenewTokenSignal)
//                                 } else {
//                                         go time.AfterFunc(renewDuration(), r.sendRenewTokenSignal)
//                                 }
//                         case <-r.disconnect:
//                                 return
//                         }
//                 }
//         }()
// }

# lisabot-hipchat

The hipchat adapter for lisabot

## Acknowledgement

A good chunk of the code is (shamelessly) taken from
[https://github.com/daneharrigan/hipchat/], which provided a very nice starting
platform that I only have to tweek some components to get it fully functional
for what I need. A couple of major components I have to tweak include:
  * authentication - it seems HipChat no longer accepts jabber:iq:auth (or
    having trouble accepting it). HipChat server will respond with "409
    not-acceptable" when trying to authenticate with jabber:iq:auth method. So
    I implemented their X-HIPCHAT-OAUTH2 SASL method. It was chosen over the
    SASL-Plain due to some additional features we get when using it as
    authentication method.
  * Instead of string substitution method, I took the XML encoder approach when
    sending out stanzas. Still not sure whether this is a better idea or not,
    since it seems more wordier than the simple string approach. But I think
    it's slightly more readable this way (or at least I thought so)


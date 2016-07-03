# priscilla-hipchat

[![Build Status](https://travis-ci.org/priscillachat/priscilla-hipchat.svg?branch=master)](https://travis-ci.org/priscillachat/priscilla-hipchat)
[![Code Climate](https://codeclimate.com/github/priscillachat/priscilla-hipchat/badges/gpa.svg)](https://codeclimate.com/github/priscillachat/priscilla-hipchat)

The hipchat adapter for Priscilla

Early stage of the development, I was hoping to use an existing xmpp-go library,
but they don't have the flexibility I need. So I pretty much have to come up
with my own.

Luckily someone else wrote a library that is specifically for hipchat,
https://github.com/daneharrigan/hipchat/, which provided a very nice starting
platform. A good chunck of the xmpp code was taken from it, though I have to
re-work some of it to make it work.

package mobilebackup

// ApplyExpiry applies an explicit import eligibility clock to privately owned
// candidates. It is not the durable expiry scheduler. Validation is whole-page
// and precedes any clearing or removal.
func (p *PreparedRowPage) ApplyExpiry(nowMS int64) (expiredMessages, expiredQuotes int, err error) {
	if p == nil || nowMS <= 0 {
		return 0, 0, ErrExpiry
	}
	for i := range p.Rows {
		row := &p.Rows[i]
		if _, _, err = MessageExpiryMS(row.Row.TimestampMS, row.Row.TTL); err != nil {
			return 0, 0, err
		}
		if _, _, err = quoteExpiry(row.Metadata.Quote); err != nil {
			return 0, 0, err
		}
	}
	retained := 0
	for i := range p.Rows {
		row := &p.Rows[i]
		expiry, declared, _ := MessageExpiryMS(row.Row.TimestampMS, row.Row.TTL)
		row.ExpiresMS, row.ExpiryDeclared = expiry, declared
		if declared && expiry <= nowMS {
			clear(row.Row.BinNet)
			row.Metadata.Clear()
			*row = PreparedRow{}
			expiredMessages++
			continue
		}
		q := row.Metadata.Quote
		qe, qd, _ := quoteExpiry(q)
		if qd && qe <= nowMS && !row.QuoteExpired {
			clear(row.Row.BinNet)
			row.Row.BinNet = nil
			row.QuoteExpired = true
			for _, value := range []*QuoteValue{&q.Message, &q.Attachment, &q.FromD} {
				clear(value.Bytes)
				*value = QuoteValue{}
			}
			expiredQuotes++
		}
		if retained != i {
			p.Rows[retained] = *row
			*row = PreparedRow{}
		}
		retained++
	}
	p.Rows = p.Rows[:retained]
	return expiredMessages, expiredQuotes, nil
}

func quoteExpiry(q *Quote) (int64, bool, error) {
	if q == nil || q.Scalars.TTL == nil || *q.Scalars.TTL == 0 {
		return 0, false, nil
	}
	if q.Scalars.Timestamp == nil {
		return 0, false, ErrExpiry
	}
	return MessageExpiryMS(*q.Scalars.Timestamp, *q.Scalars.TTL)
}

-- +goose Up

CREATE TABLE companies (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text        NOT NULL,
    slug            text        NOT NULL UNIQUE,
    website         text        NOT NULL DEFAULT '',
    careers_url     text        NOT NULL DEFAULT '',
    preferred_email text,
    notes           text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE contacts (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL,
    email       text        NOT NULL DEFAULT '',
    linkedin    text        NOT NULL DEFAULT '',
    role        text        NOT NULL DEFAULT '',
    company_id  uuid        REFERENCES companies(id) ON DELETE SET NULL,
    notes       text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE opportunities (
    id                    uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id            uuid        NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    role_title            text,
    location              text        NOT NULL DEFAULT '',
    office_days_per_week  int         NOT NULL,
    source                text        NOT NULL,
    source_detail         text        NOT NULL DEFAULT '',
    priority              text        NOT NULL,
    latest_status         text        NOT NULL,
    archived_at           timestamptz,
    archive_reason        text,
    notes                 text        NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT opportunities_office_days_chk
        CHECK (office_days_per_week BETWEEN 0 AND 5),
    CONSTRAINT opportunities_source_chk
        CHECK (source IN (
            'outbound','inbound_inhouse_recruiter','inbound_external_recruiter',
            'inbound_founder','inbound_employee','referral','network','other'
        )),
    CONSTRAINT opportunities_priority_chk
        CHECK (priority IN ('low','normal','high')),
    CONSTRAINT opportunities_latest_status_chk
        CHECK (latest_status IN (
            'watching','exploring','applied','in_progress','offer',
            'accepted','dormant','archived'
        ))
);

CREATE TABLE opportunity_contacts (
    opportunity_id uuid NOT NULL REFERENCES opportunities(id) ON DELETE CASCADE,
    contact_id     uuid NOT NULL REFERENCES contacts(id)      ON DELETE CASCADE,
    relationship   text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (opportunity_id, contact_id, relationship),
    CONSTRAINT opportunity_contacts_relationship_chk
        CHECK (relationship IN ('recruiter','hiring_manager','referrer','interviewer','other'))
);

CREATE TABLE applications (
    id                      uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    opportunity_id          uuid        NOT NULL REFERENCES opportunities(id) ON DELETE RESTRICT,
    applied_at              timestamptz,
    applied_with_email      text        NOT NULL DEFAULT '',
    status                  text        NOT NULL,
    archived_at             timestamptz,
    archive_reason_category text,
    archive_reason          text,
    follow_up_blocked       boolean     NOT NULL DEFAULT false,
    last_followed_up_at     timestamptz,
    notes                   text        NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT applications_status_chk
        CHECK (status IN (
            'applied','in_progress','offer','accepted',
            'rejected','declined','withdrawn'
        )),
    CONSTRAINT applications_archive_reason_chk CHECK (
        CASE status
            WHEN 'rejected' THEN
                archive_reason_category IN
                    ('fit_mismatch','team_preference','role_change','process_ended','other')
            WHEN 'declined' THEN
                archive_reason_category IN
                    ('compensation','scope','team_fit','timing','other')
            WHEN 'withdrawn' THEN
                archive_reason_category IN
                    ('compensation','scope','team_fit','timing','other')
            ELSE archive_reason_category IS NULL
        END
    ),
    -- Anchor for composite FK from events(opportunity_id, application_id).
    CONSTRAINT applications_opp_id_unique UNIQUE (opportunity_id, id)
);

CREATE UNIQUE INDEX uq_active_app_per_opportunity
    ON applications (opportunity_id)
    WHERE status IN ('applied','in_progress','offer');

CREATE TABLE application_stages (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id     uuid        NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    order_index        int         NOT NULL,
    label              text        NOT NULL,
    expectation_notes  text        NOT NULL DEFAULT '',
    prep_notes         text        NOT NULL DEFAULT '',
    scheduled_at       timestamptz,
    completed_at       timestamptz,
    outcome            text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, order_index),
    CONSTRAINT application_stages_outcome_chk
        CHECK (outcome IS NULL OR outcome IN ('passed','failed','skipped'))
);

CREATE TABLE events (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    opportunity_id  uuid        NOT NULL REFERENCES opportunities(id) ON DELETE CASCADE,
    application_id  uuid,
    stage_id        uuid        REFERENCES application_stages(id) ON DELETE SET NULL,
    kind            text        NOT NULL,
    occurred_at     timestamptz NOT NULL,
    label           text,
    notes           text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT events_kind_chk CHECK (kind IN (
        'added','exploring','applied',
        'screen','technical','system_design','behavioral','onsite',
        'stage_scheduled','stage_completed',
        'offer','counter','accepted','rejected','declined','withdrawn',
        'archived','reopened','note','follow_up','custom'
    )),
    -- label is only meaningful when kind = 'custom'; require it then,
    -- forbid it otherwise.
    CONSTRAINT events_label_only_for_custom_chk CHECK (
        (kind = 'custom' AND label IS NOT NULL)
        OR (kind <> 'custom' AND label IS NULL)
    ),
    -- Composite FK: when application_id is set, it must belong to the
    -- same opportunity as the event. NULL application_id short-circuits
    -- the check under MATCH SIMPLE semantics, which is what we want for
    -- pre-application events.
    CONSTRAINT events_application_belongs_to_opportunity_fk
        FOREIGN KEY (opportunity_id, application_id)
        REFERENCES applications (opportunity_id, id)
        ON DELETE CASCADE
);

CREATE TABLE compensations (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    opportunity_id  uuid        REFERENCES opportunities(id) ON DELETE CASCADE,
    application_id  uuid        REFERENCES applications(id)  ON DELETE CASCADE,
    kind            text        NOT NULL,
    base_minor      bigint      NOT NULL,
    bonus_minor     bigint,
    equity_notes    text        NOT NULL DEFAULT '',
    vesting_notes   text        NOT NULL DEFAULT '',
    currency        text        NOT NULL,
    notes           text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT compensations_kind_chk
        CHECK (kind IN ('target','listed','offered','counter','accepted')),
    CONSTRAINT compensations_currency_chk
        CHECK (currency IN ('EUR','USD')),
    -- Exactly one of opportunity_id / application_id is set.
    CONSTRAINT compensations_target_xor_chk
        CHECK ((opportunity_id IS NULL) <> (application_id IS NULL))
);

COMMENT ON COLUMN compensations.base_minor IS
    'Base salary in the currency''s minor unit (e.g. cents for EUR/USD). Integer to avoid float rounding.';
COMMENT ON COLUMN compensations.bonus_minor IS
    'Bonus amount in the currency''s minor unit (e.g. cents for EUR/USD). Integer to avoid float rounding.';

-- +goose Down

DROP TABLE IF EXISTS compensations;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS application_stages;
DROP TABLE IF EXISTS applications;
DROP TABLE IF EXISTS opportunity_contacts;
DROP TABLE IF EXISTS opportunities;
DROP TABLE IF EXISTS contacts;
DROP TABLE IF EXISTS companies;

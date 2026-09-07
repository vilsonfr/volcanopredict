-- +goose Up
-- Task 1.3 (registro-de-fontes spec, V0.2). Até aqui `update_cadence`
-- descrevia com que frequência a FONTE publica. A ingestão automática
-- introduz uma segunda cadência, distinta: com que frequência NÓS
-- consultamos. Confundir as duas leva a consultar de hora em hora uma
-- fonte que publica uma vez por ano.
--
-- E introduz uma segunda razão para uma fonte não poder ser usada, que
-- não é licença: a fonte PROIBIR coleta automatizada. O PVMBG é o caso
-- concreto — robots.txt com `Disallow: /`. Licença permissiva não
-- destrava uma fonte que pediu para não ser coletada, então o bloqueio
-- precisa ser um campo próprio e não um license vazio.

ALTER TABLE data_sources
    -- Com que frequência este sistema consulta a fonte. NULL = não é
    -- coletada automaticamente (o GVP, por exemplo, é troca manual de
    -- snapshot).
    ADD COLUMN collection_cadence TEXT,
    -- Política de coleta declarada pela própria fonte, quando existir:
    -- o conteúdo relevante do robots.txt, dos termos, ou do contrato.
    ADD COLUMN collection_policy TEXT,
    -- Preenchido apenas quando a fonte é proibida de ser coletada. A
    -- presença deste campo é o bloqueio; o texto é a evidência.
    ADD COLUMN blocked_reason TEXT;

-- Uma fonte bloqueada não pode ser habilitada, mesmo com licença
-- preenchida. Sem esta restrição, "temos licença" seria suficiente para
-- ligar uma fonte que pediu para não ser coletada.
ALTER TABLE data_sources
    ADD CONSTRAINT data_sources_enabled_requires_not_blocked
    CHECK (NOT enabled OR blocked_reason IS NULL OR blocked_reason = '');

-- +goose Down
ALTER TABLE data_sources
    DROP CONSTRAINT IF EXISTS data_sources_enabled_requires_not_blocked,
    DROP COLUMN IF EXISTS collection_cadence,
    DROP COLUMN IF EXISTS collection_policy,
    DROP COLUMN IF EXISTS blocked_reason;

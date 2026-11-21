<?xml version="1.0" encoding="UTF-8"?>
<!--
  Aumbry XML protocol v1.0 (docs/prd_spec_v1.0/prd.md section 6.2) as an ISO Schematron schema.
  Every assert/report message is "CODE|field|human readable text"; the validator splits it into
  ValidationError.Code, .Field and .Message. The line comes from the offending node.
  Checks that need the filesystem or net/url (figure src, http(s) URLs) live in validator.go.
-->
<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xpath3">
  <pattern id="root">
    <rule context="/*">
      <assert test="self::document">INVALID_ROOT|document|root element must be &lt;document&gt;, got &lt;<value-of select="name()"/>&gt;</assert>
    </rule>
  </pattern>

  <pattern id="document">
    <rule context="/document">
      <assert test="@version = '1.0'">UNSUPPORTED_VERSION|document.version|version must be "1.0"</assert>
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|document|&lt;document&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
      <assert test="meta">MISSING_META|document.meta|&lt;meta&gt; is required</assert>
      <assert test="body">MISSING_BODY|document.body|&lt;body&gt; is required</assert>
    </rule>
    <rule context="/document/*[not(self::meta or self::body)]">
      <report test="true()">UNKNOWN_TAG|document|unexpected &lt;<value-of select="name()"/>&gt; inside &lt;document&gt;</report>
    </rule>
  </pattern>

  <pattern id="meta">
    <rule context="/document/meta">
      <assert test="normalize-space(title[1]) != ''">REQUIRED_FIELD|meta.title|meta.title is required</assert>
      <assert test="normalize-space(type[1]) != ''">REQUIRED_FIELD|meta.type|meta.type is required</assert>
      <assert test="normalize-space(date[1]) != ''">REQUIRED_FIELD|meta.date|meta.date is required</assert>
      <assert test="normalize-space(summary[1]) != ''">REQUIRED_FIELD|meta.summary|meta.summary is required</assert>
      <assert test="normalize-space(source-agent[1]) != ''">REQUIRED_FIELD|meta.source-agent|meta.source-agent is required</assert>
    </rule>
    <rule context="/document/meta/type">
      <assert test="normalize-space(.) = ('', 'report', 'post')">INVALID_TYPE|meta.type|type must be report or post</assert>
    </rule>
    <rule context="/document/meta/date">
      <assert test="normalize-space(.) = '' or (matches(normalize-space(.), '^\d{4}-\d{2}-\d{2}$') and normalize-space(.) castable as xs:date)">INVALID_DATE|meta.date|date must use YYYY-MM-DD format</assert>
    </rule>
    <rule context="/document/meta/tags/*">
      <assert test="self::tag and normalize-space(.) != ''">INVALID_TAG|meta.tags|&lt;tags&gt; may only contain non-empty &lt;tag&gt; elements</assert>
    </rule>
    <rule context="/document/meta/related-ids/*">
      <assert test="self::id and matches(normalize-space(.), '^[A-Za-z0-9_-]{12}$')">INVALID_ID|meta.related-ids|related-ids may only contain &lt;id&gt; elements holding a 12-character item id</assert>
    </rule>
  </pattern>

  <pattern id="containers">
    <rule context="/document/body">
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|body|&lt;body&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
    </rule>
    <rule context="/document/body/*[not(self::section or self::sources)]">
      <report test="true()">UNKNOWN_TAG|body|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;body&gt;</report>
    </rule>
    <rule context="/document/body/section | /document/body/section/collapse">
      <assert test="normalize-space(@title) != ''">MISSING_ATTR|<value-of select="name()"/>.title|&lt;<value-of select="name()"/>&gt; requires the "title" attribute</assert>
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|<value-of select="name()"/>|&lt;<value-of select="name()"/>&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
    </rule>
    <rule context="/document/body/section/*[not(self::md or self::figure or self::paper-card or self::repo-card or self::compare-table or self::callout or self::collapse or self::sources)]">
      <report test="true()">UNKNOWN_TAG|section|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;section&gt;</report>
    </rule>
    <rule context="/document/body/section/collapse/*[not(self::md or self::figure)]">
      <report test="true()">UNKNOWN_TAG|collapse|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;collapse&gt;</report>
    </rule>
  </pattern>

  <pattern id="leaves">
    <rule context="/document/body/section/*[self::md or self::figure or self::paper-card or self::repo-card or self::callout]/* | /document/body/section/collapse/*[self::md or self::figure]/*">
      <report test="true()">UNKNOWN_TAG|<value-of select="name(..)"/>|&lt;<value-of select="name(..)"/>&gt; cannot contain child elements</report>
    </rule>
  </pattern>

  <pattern id="blocks">
    <rule context="/document/body/section/figure | /document/body/section/collapse/figure">
      <assert test="normalize-space(@src) != ''">MISSING_ATTR|figure.src|&lt;figure&gt; requires the "src" attribute</assert>
    </rule>
    <rule context="/document/body/section/paper-card">
      <assert test="normalize-space(@title) != ''">MISSING_ATTR|paper-card.title|&lt;paper-card&gt; requires the "title" attribute</assert>
      <assert test="normalize-space(@url) != ''">MISSING_ATTR|paper-card.url|&lt;paper-card&gt; requires the "url" attribute</assert>
      <assert test="not(@related-id) or matches(@related-id, '^[A-Za-z0-9_-]{12}$')">INVALID_ID|paper-card.related-id|related-id must be a 12-character item id</assert>
    </rule>
    <rule context="/document/body/section/repo-card">
      <assert test="normalize-space(@name) != ''">MISSING_ATTR|repo-card.name|&lt;repo-card&gt; requires the "name" attribute</assert>
      <assert test="normalize-space(@url) != ''">MISSING_ATTR|repo-card.url|&lt;repo-card&gt; requires the "url" attribute</assert>
    </rule>
    <rule context="/document/body/section/callout">
      <assert test="@type = ('info', 'warn', 'insight')">INVALID_ATTR|callout.type|callout type must be info, warn or insight</assert>
    </rule>
  </pattern>

  <pattern id="compare-table">
    <rule context="/document/body/section/compare-table">
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|compare-table|&lt;compare-table&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
      <assert test="columns/col">TABLE_NO_COLUMNS|compare-table.columns|&lt;compare-table&gt; requires &lt;columns&gt; with at least one &lt;col&gt;</assert>
      <assert test="row">TABLE_NO_ROWS|compare-table.row|&lt;compare-table&gt; requires at least one &lt;row&gt;</assert>
    </rule>
    <rule context="/document/body/section/compare-table/*[not(self::columns or self::row)]">
      <report test="true()">UNKNOWN_TAG|compare-table|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;compare-table&gt;</report>
    </rule>
    <rule context="/document/body/section/compare-table/columns/*[not(self::col)]">
      <report test="true()">UNKNOWN_TAG|compare-table.columns|&lt;columns&gt; may only contain &lt;col&gt;</report>
    </rule>
    <rule context="/document/body/section/compare-table/row/*[not(self::cell)]">
      <report test="true()">UNKNOWN_TAG|compare-table.row|&lt;row&gt; may only contain &lt;cell&gt;</report>
    </rule>
    <rule context="/document/body/section/compare-table/row">
      <assert test="not(../columns/col) or count(cell) = count(../columns/col)">TABLE_COL_MISMATCH|compare-table.row[<value-of select="count(preceding-sibling::row) + 1"/>]|expected <value-of select="count(../columns/col)"/> cells to match columns, got <value-of select="count(cell)"/></assert>
    </rule>
  </pattern>

  <pattern id="sources">
    <rule context="/document/body/sources | /document/body/section/sources">
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|sources|&lt;sources&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
      <assert test="source">MISSING_SOURCE|sources|&lt;sources&gt; requires at least one &lt;source&gt;</assert>
    </rule>
    <rule context="/document/body/sources/*[not(self::source)] | /document/body/section/sources/*[not(self::source)]">
      <report test="true()">UNKNOWN_TAG|sources|&lt;sources&gt; may only contain &lt;source&gt;</report>
    </rule>
    <rule context="/document/body/sources/source | /document/body/section/sources/source">
      <assert test="normalize-space(@title) != ''">MISSING_ATTR|source.title|&lt;source&gt; requires the "title" attribute</assert>
      <assert test="normalize-space(@url) != ''">MISSING_ATTR|source.url|&lt;source&gt; requires the "url" attribute</assert>
    </rule>
  </pattern>
</schema>

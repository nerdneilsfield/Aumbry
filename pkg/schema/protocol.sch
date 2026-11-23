<?xml version="1.0" encoding="UTF-8"?>
<!--
  Aumbry XML protocol (v1.1 and v1.2) as an ISO Schematron schema.
  Every assert/report message is "CODE|field|human readable text[|optional hint]"; the validator splits it
  into ValidationError.Code, .Field, .Message and .Hint. The line comes from the offending node.
  Checks requiring JSON Schema, filesystem, net/url, rune counting or shared id namespaces live in Go.
-->
<schema xmlns="http://purl.oclc.org/dsdl/schematron" queryBinding="xpath3">
  <pattern id="root">
    <rule context="/*">
      <assert test="self::document">INVALID_ROOT|document|root element must be &lt;document&gt;, got &lt;<value-of select="name()"/>&gt;</assert>
    </rule>
  </pattern>

  <pattern id="document">
    <rule context="/document">
      <assert test="@version = ('1.1', '1.2')">UNSUPPORTED_VERSION|document.version|version must be "1.1" or "1.2"|set &lt;document version="1.2"&gt; (or "1.1" for legacy documents without v1.2 elements)</assert>
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|document|&lt;document&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
      <assert test="meta">MISSING_META|document.meta|&lt;meta&gt; is required</assert>
      <assert test="body">MISSING_BODY|document.body|&lt;body&gt; is required</assert>
      <assert test="count(meta) le 1">DUPLICATE_META|document.meta|&lt;document&gt; must contain exactly one &lt;meta&gt;</assert>
      <assert test="count(body) le 1">DUPLICATE_BODY|document.body|&lt;document&gt; must contain exactly one &lt;body&gt;</assert>
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
    <rule context="/document/body | /document/body//stages/stage">
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|<value-of select="name()"/>|&lt;<value-of select="name()"/>&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
    </rule>
    <rule context="/document/body/*[not(local-name() = ('section', 'sources', 'md', 'figure', 'audio', 'paper-card', 'repo-card', 'compare-table', 'callout', 'collapse', 'claim', 'equation', 'mermaid', 'chart', 'metrics', 'steps', 'tabs', 'timeline', 'glossary', 'references', 'figure-group', 'code-block', 'quote', 'checklist', 'search-log', 'stages', 'quiz', 'next-steps', 'tldr', 'news-item'))] | /document/body//section/*[not(local-name() = ('section', 'sources', 'md', 'figure', 'audio', 'paper-card', 'repo-card', 'compare-table', 'callout', 'collapse', 'claim', 'equation', 'mermaid', 'chart', 'metrics', 'steps', 'tabs', 'timeline', 'glossary', 'references', 'figure-group', 'code-block', 'quote', 'checklist', 'search-log', 'stages', 'quiz', 'next-steps', 'tldr', 'news-item'))]">
      <report test="true()">UNKNOWN_TAG|<value-of select="name(..)"/>|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;<value-of select="name(..)"/>&gt;</report>
    </rule>
    <rule context="/document/body//stages/stage/*[not(local-name() = ('section', 'sources', 'md', 'figure', 'audio', 'paper-card', 'repo-card', 'compare-table', 'callout', 'collapse', 'claim', 'equation', 'mermaid', 'chart', 'metrics', 'steps', 'tabs', 'timeline', 'glossary', 'references', 'figure-group', 'code-block', 'quote', 'checklist', 'search-log', 'stages', 'quiz', 'next-steps', 'tldr', 'news-item'))]">
      <report test="true()">UNKNOWN_TAG|stage|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;stage&gt;</report>
    </rule>
    <rule context="/document/body//collapse/*[not(local-name() = ('md', 'figure', 'audio', 'callout', 'claim', 'equation', 'mermaid', 'chart', 'code-block', 'quote', 'search-log', 'tldr'))]">
      <report test="true()">UNKNOWN_TAG|collapse|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;collapse&gt;</report>
    </rule>
    <rule context="/document/body//tab/*[not(local-name() = ('md', 'figure', 'audio', 'callout', 'claim', 'equation', 'mermaid', 'chart', 'code-block', 'quote', 'tldr'))] | /document/body//step/*[not(local-name() = ('md', 'figure', 'audio', 'callout', 'claim', 'equation', 'mermaid', 'chart', 'code-block', 'quote', 'tldr'))]">
      <report test="true()">UNKNOWN_TAG|<value-of select="name(..)"/>|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;<value-of select="name(..)"/>&gt;</report>
    </rule>
  </pattern>

  <pattern id="v12-features">
    <rule context="/document[@version = '1.1']/body//audio">
      <report test="true()">INVALID_AUDIO|audio|&lt;audio&gt; requires &lt;document version="1.2"&gt;|upgrade the root element to &lt;document version="1.2"&gt;</report>
    </rule>
  </pattern>

  <!-- Separate pattern on purpose: within one pattern a node fires only the first rule whose context matches it,
       so a <collapse>/<section> nested in a leaf-only container would never reach the child rules above. -->
  <pattern id="titled-containers">
    <rule context="/document/body//section | /document/body//collapse | /document/body//tabs/tab | /document/body//steps/step">
      <assert test="normalize-space(@title) != ''">MISSING_ATTR|<value-of select="name()"/>.title|&lt;<value-of select="name()"/>&gt; requires the "title" attribute</assert>
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|<value-of select="name()"/>|&lt;<value-of select="name()"/>&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
    </rule>
  </pattern>

  <pattern id="leaves">
    <rule context="/document/body//*[self::md or self::figure or self::paper-card or self::repo-card or self::callout or self::claim or self::equation or self::mermaid or self::chart or self::code-block or self::quote or self::source or self::cell or self::col]/*">
      <report test="true()">UNKNOWN_TAG|<value-of select="name(..)"/>|&lt;<value-of select="name(..)"/>&gt; cannot contain child elements</report>
    </rule>
  </pattern>

  <pattern id="blocks">
    <rule context="/document/body//figure">
      <assert test="normalize-space(@src) != ''">MISSING_ATTR|figure.src|&lt;figure&gt; requires the "src" attribute</assert>
    </rule>
    <rule context="/document/body//paper-card">
      <assert test="normalize-space(@title) != ''">MISSING_ATTR|paper-card.title|&lt;paper-card&gt; requires the "title" attribute</assert>
      <assert test="normalize-space(@url) != ''">MISSING_ATTR|paper-card.url|&lt;paper-card&gt; requires the "url" attribute</assert>
      <assert test="not(@related-id) or matches(@related-id, '^[A-Za-z0-9_-]{12}$')">INVALID_ID|paper-card.related-id|related-id must be a 12-character item id</assert>
    </rule>
    <rule context="/document/body//repo-card">
      <assert test="normalize-space(@name) != ''">MISSING_ATTR|repo-card.name|&lt;repo-card&gt; requires the "name" attribute</assert>
      <assert test="normalize-space(@url) != ''">MISSING_ATTR|repo-card.url|&lt;repo-card&gt; requires the "url" attribute</assert>
    </rule>
    <rule context="/document/body//callout">
      <assert test="@type = ('info', 'warn', 'insight')">INVALID_ATTR|callout.type|callout type must be info, warn or insight</assert>
    </rule>
  </pattern>

  <pattern id="compare-table">
    <rule context="/document/body//compare-table">
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|compare-table|&lt;compare-table&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
      <assert test="columns/col">TABLE_NO_COLUMNS|compare-table.columns|&lt;compare-table&gt; requires &lt;columns&gt; with at least one &lt;col&gt;</assert>
      <assert test="row">TABLE_NO_ROWS|compare-table.row|&lt;compare-table&gt; requires at least one &lt;row&gt;</assert>
    </rule>
    <rule context="/document/body//compare-table/*[not(self::columns or self::row)]">
      <report test="true()">UNKNOWN_TAG|compare-table|&lt;<value-of select="name()"/>&gt; is not allowed inside &lt;compare-table&gt;</report>
    </rule>
    <rule context="/document/body//compare-table/columns/*[not(self::col)]">
      <report test="true()">UNKNOWN_TAG|compare-table.columns|&lt;columns&gt; may only contain &lt;col&gt;</report>
    </rule>
    <rule context="/document/body//compare-table/row/*[not(self::cell)]">
      <report test="true()">UNKNOWN_TAG|compare-table.row|&lt;row&gt; may only contain &lt;cell&gt;</report>
    </rule>
    <rule context="/document/body//compare-table/row">
      <assert test="not(../columns/col) or count(cell) = count(../columns/col)">TABLE_COL_MISMATCH|compare-table.row[<value-of select="count(preceding-sibling::row) + 1"/>]|expected <value-of select="count(../columns/col)"/> cells to match columns, got <value-of select="count(cell)"/></assert>
    </rule>
  </pattern>

  <pattern id="sources">
    <rule context="/document/body//sources">
      <assert test="not(text()[normalize-space()])">UNEXPECTED_TEXT|sources|&lt;sources&gt; cannot contain text directly; wrap it in &lt;md&gt;</assert>
      <assert test="source">MISSING_SOURCE|sources|&lt;sources&gt; requires at least one &lt;source&gt;</assert>
    </rule>
    <rule context="/document/body//sources/*[not(self::source)]">
      <report test="true()">UNKNOWN_TAG|sources|&lt;sources&gt; may only contain &lt;source&gt;</report>
    </rule>
    <rule context="/document/body//sources/source">
      <assert test="normalize-space(@title) != ''">MISSING_ATTR|source.title|&lt;source&gt; requires the "title" attribute</assert>
      <assert test="normalize-space(@url) != ''">MISSING_ATTR|source.url|&lt;source&gt; requires the "url" attribute</assert>
    </rule>
  </pattern>
</schema>

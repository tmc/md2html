(function() {
    function parseBundle() {
        var tag = document.getElementById('md-jsonspec-schemas');
        if (!tag) return null;
        try {
            return JSON.parse(tag.textContent);
        } catch (e) {
            console.warn('jsonspec: bundle parse failed', e);
            return null;
        }
    }

    function formatField(name, field) {
        if (!field) return name + ' (undocumented)';
        var lines = [name];
        var head = [];
        if (field.type) head.push(field.type);
        if (field.format) head.push('format: ' + field.format);
        head.push(field.required ? 'required' : 'optional');
        lines.push('  ' + head.join(' · '));
        if (field.description) {
            lines.push('');
            lines.push(field.description);
        }
        if (field.enum && field.enum.length) {
            lines.push('');
            lines.push('one of: ' + field.enum.map(String).join(' | '));
        }
        if (field.pattern) {
            lines.push('');
            lines.push('pattern: ' + field.pattern);
        }
        return lines.join('\n');
    }

    function formatEnumValue(field) {
        if (!field || !field.enum || !field.enum.length) return null;
        return 'one of: ' + field.enum.map(String).join(' | ');
    }

    function unquote(s) {
        if (!s) return '';
        if (s.length >= 2 && s.charAt(0) === '"' && s.charAt(s.length - 1) === '"') {
            return s.substring(1, s.length - 1);
        }
        return s;
    }

    function enrichBlock(block, schema) {
        if (!schema || !schema.fields) return;
        var code = block.querySelector('pre code');
        if (!code) return;
        var spans = code.querySelectorAll('span.nt, span.s2');
        var lastKeyField = null;
        for (var i = 0; i < spans.length; i++) {
            var sp = spans[i];
            if (sp.classList.contains('nt')) {
                var name = unquote(sp.textContent);
                var field = schema.fields[name];
                lastKeyField = field || null;
                sp.setAttribute('title', formatField(name, field));
                sp.classList.add('md-jsonspec-key');
                if (field && field.required) sp.classList.add('md-jsonspec-required');
            } else if (sp.classList.contains('s2')) {
                var enumTip = formatEnumValue(lastKeyField);
                if (enumTip) {
                    sp.setAttribute('title', enumTip);
                    sp.classList.add('md-jsonspec-enum');
                }
                lastKeyField = null;
            }
        }
    }

    function run() {
        var bundle = parseBundle();
        if (!bundle || !bundle.schemas) return;
        var blocks = document.querySelectorAll('.md-jsonspec');
        for (var i = 0; i < blocks.length; i++) {
            var block = blocks[i];
            var type = block.getAttribute('data-schema-type');
            if (type) enrichBlock(block, bundle.schemas[type]);
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', run);
    } else {
        run();
    }
})();

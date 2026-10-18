package browser

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// Field is one question on an application step, as the page presents it.
type Field struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"` // text email tel number url date textarea select radio checkbox file combobox
	Label    string   `json:"label"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	Value    string   `json:"value"`
	Accept   string   `json:"accept,omitempty"`
}

type Button struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Kind string `json:"kind"` // submit review next apply other
}

type Step struct {
	Fields  []Field  `json:"fields"`
	Buttons []Button `json:"buttons"`
	Errors  []string `json:"errors"`
	Text    string   `json:"text"`
}

// ReadStep collects the visible form fields, buttons and validation errors
// inside the first element matching one of roots (or the whole document).
func ReadStep(p *rod.Page, roots ...string) (Step, error) {
	var s Step
	err := Eval(p, readStepJS, &s, strings.Join(roots, ", "))
	return s, err
}

const readStepJS = `(rootSel) => {
	let root = document;
	if (rootSel) { const cands = [...document.querySelectorAll(rootSel)].filter(e => e.getBoundingClientRect().height > 0); if (cands.length) root = cands[cands.length - 1]; }
	const seq = () => (window.__pfSeq = (window.__pfSeq || 0) + 1);
	const mark = (el, attr) => { if (!el.dataset[attr]) el.dataset[attr] = (attr === 'pfId' ? 'f' : 'b') + seq(); return el.dataset[attr]; };
	const clean = t => (t || '').replace(/\s+/g, ' ').trim();
	const shown = el => {
		const r = el.getBoundingClientRect(); const s = getComputedStyle(el);
		return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none';
	};
	const textOf = el => clean(el.innerText || el.textContent);
	const labelOf = el => {
		if (el.id) { const l = document.querySelector('label[for="' + CSS.escape(el.id) + '"]'); if (l && textOf(l)) return textOf(l); }
		const lb = el.getAttribute('aria-labelledby');
		if (lb) { const t = clean(lb.split(/\s+/).map(id => { const n = document.getElementById(id); return n ? textOf(n) : ''; }).join(' ')); if (t) return t; }
		const al = el.getAttribute('aria-label'); if (al) return clean(al);
		const wrap = el.closest('label'); if (wrap && textOf(wrap)) return textOf(wrap);
		let c = el.parentElement;
		for (let i = 0; i < 5 && c && c !== root; i++, c = c.parentElement) {
			const lab = [...c.querySelectorAll('label, legend, [class*="label" i], [class*="question" i], h3, h4, p')].find(n => !n.contains(el) && textOf(n) && textOf(n).length < 400);
			if (lab) return textOf(lab);
		}
		return clean(el.placeholder || el.name || '');
	};
	const isReq = (el, label) => el.required || el.getAttribute('aria-required') === 'true' || /\*\s*$/.test(label) || /\(required\)|\(wajib\)/i.test(label);
	const fields = [], groups = {};
	for (const el of root.querySelectorAll('input, select, textarea')) {
		const type = (el.type || el.tagName).toLowerCase();
		if (['hidden', 'submit', 'button', 'image', 'reset', 'search', 'password'].includes(type) || el.disabled) continue;
		if (type === 'radio' || type === 'checkbox') {
			const holder = el.closest('label') || el.parentElement;
			if (!shown(el) && !(holder && shown(holder))) continue;
		} else if (type !== 'file' && !shown(el)) continue;
		if (el.readOnly && type !== 'file') continue;
		if (type === 'radio') {
			const box = el.closest('fieldset') || el.closest('[role="radiogroup"]') || el.parentElement.parentElement;
			const key = el.name || mark(box, 'pfId');
			let g = groups[key];
			if (!g) {
				const leg = box.querySelector('legend') || box.querySelector('[class*="label" i], [class*="title" i], [class*="question" i]');
				const label = leg ? textOf(leg) : labelOf(box);
				g = groups[key] = { id: 'g' + seq(), kind: 'radio', label, required: false, options: [], value: '' };
				fields.push(g);
			}
			el.dataset.pfGroup = g.id;
			const opt = labelOf(el);
			el.dataset.pfOption = opt;
			g.options.push(opt);
			if (el.required) g.required = true;
			if (el.checked) g.value = opt;
			continue;
		}
		const label = labelOf(el);
		const f = { id: mark(el, 'pfId'), kind: type, label, required: isReq(el, label), options: [], value: '' };
		if (el.tagName === 'SELECT') {
			f.kind = 'select';
			f.options = [...el.options].filter(o => o.value !== '' && !/^(select|pilih|choose|--)/i.test(clean(o.text))).map(o => clean(o.text));
			const sel = el.options[el.selectedIndex];
			f.value = sel && sel.value !== '' ? clean(sel.text) : '';
		} else if (type === 'checkbox') {
			f.options = ['Yes', 'No']; f.value = el.checked ? 'Yes' : 'No';
		} else if (type === 'file') {
			f.accept = el.accept || ''; f.value = el.files && el.files.length ? el.files[0].name : '';
		} else {
			if (el.tagName === 'TEXTAREA') f.kind = 'textarea';
			else if (el.getAttribute('role') === 'combobox' || el.getAttribute('aria-autocomplete') || el.list) f.kind = 'combobox';
			else if (!['email', 'tel', 'number', 'url', 'date'].includes(type)) f.kind = 'text';
			f.value = el.value || '';
		}
		fields.push(f);
	}
	const buttons = [];
	const kindOf = t => {
		t = t.toLowerCase();
		if (/^(submit|send)( (your )?application)?$|submit application|kirim lamaran|kirim$|^kirim |^lamar sekarang$|submit your application|send application/.test(t)) return 'submit';
		if (/^review|tinjau|periksa/.test(t)) return 'review';
		if (/^(next|continue|lanjut|lanjutkan|selanjutnya|berikutnya|save and continue|simpan dan lanjut)/.test(t)) return 'next';
		if (/easy apply|lamar mudah|^apply|^lamar|apply now|quick apply|lamar cepat|apply for this job/.test(t)) return 'apply';
		return 'other';
	};
	for (const el of root.querySelectorAll('button, [role="button"], input[type="submit"], a.btn, a[class*="button" i]')) {
		if (!shown(el) || el.disabled || el.getAttribute('aria-disabled') === 'true') continue;
		const text = clean(el.innerText || el.value || el.getAttribute('aria-label'));
		if (!text || text.length > 60) continue;
		buttons.push({ id: mark(el, 'pfBtn'), text, kind: kindOf(text) });
	}
	const errors = [...root.querySelectorAll('[role="alert"], [aria-live="assertive"], .artdeco-inline-feedback--error, [class*="error" i], [class*="invalid" i]')]
		.filter(shown).map(textOf).filter(t => t && t.length < 300);
	return { fields, buttons, errors: [...new Set(errors)].slice(0, 10), text: textOf(root === document ? document.body : root).slice(0, 3000) };
}`

// Fill sets one field. value is the option text for select/radio, "Yes"/"No"
// for checkboxes and a local path for file inputs.
func Fill(p *rod.Page, f Field, value string) error {
	if f.Kind == "radio" {
		return clickOption(p, f.ID, value)
	}
	el, err := p.Timeout(5 * time.Second).Element(fmt.Sprintf(`[data-pf-id=%q]`, f.ID))
	if err != nil {
		return fmt.Errorf("field %q is gone", f.Label)
	}
	el = el.Timeout(15 * time.Second)
	_ = el.ScrollIntoView()
	switch f.Kind {
	case "file":
		return el.SetFiles([]string{value})
	case "select":
		var ok bool
		if err := evalOn(el, `function (want) {
			const norm = s => (s || '').replace(/\s+/g, ' ').trim().toLowerCase();
			const w = norm(want);
			const opts = [...this.options];
			const o = opts.find(o => norm(o.text) === w) || opts.find(o => norm(o.text).includes(w) || (w && w.includes(norm(o.text)) && norm(o.text)));
			if (!o) return false;
			const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value').set;
			setter.call(this, o.value);
			this.dispatchEvent(new Event('input', { bubbles: true }));
			this.dispatchEvent(new Event('change', { bubbles: true }));
			return true;
		}`, &ok, value); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%q is not an option for %q", value, f.Label)
		}
		return nil
	case "checkbox":
		want := strings.EqualFold(value, "yes") || strings.EqualFold(value, "true") || strings.EqualFold(value, "ya")
		var checked bool
		if err := evalOn(el, `function () { return this.checked; }`, &checked); err != nil {
			return err
		}
		if checked != want {
			return evalOn(el, `function () { this.click(); }`, nil)
		}
		return nil
	}

	// Text-like: clear with the native setter (so React sees it), then type.
	if err := el.Focus(); err != nil {
		return err
	}
	if err := evalOn(el, `function () {
		const proto = this.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
		Object.getOwnPropertyDescriptor(proto, 'value').set.call(this, '');
		this.dispatchEvent(new Event('input', { bubbles: true }));
	}`, nil); err != nil {
		return err
	}
	if err := el.Input(value); err != nil {
		return err
	}
	if f.Kind == "combobox" {
		// Typeaheads (city, school…) need a suggestion picked.
		time.Sleep(1500 * time.Millisecond)
		_ = p.Keyboard.Type(input.ArrowDown)
		time.Sleep(300 * time.Millisecond)
		_ = p.Keyboard.Type(input.Enter)
	}
	return evalOn(el, `function () { this.dispatchEvent(new Event('change', { bubbles: true })); this.blur(); }`, nil)
}

func clickOption(p *rod.Page, group, option string) error {
	var ok bool
	err := Eval(p, `(group, want) => {
		const norm = s => (s || '').replace(/\s+/g, ' ').trim().toLowerCase();
		const w = norm(want);
		const radios = [...document.querySelectorAll('[data-pf-group="' + group + '"]')];
		const r = radios.find(r => norm(r.dataset.pfOption) === w) || radios.find(r => norm(r.dataset.pfOption).includes(w));
		if (!r) return false;
		(r.closest('label') || r).click();
		if (!r.checked) r.click();
		return true;
	}`, &ok, group, option)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%q is not one of the choices", option)
	}
	return nil
}

// Click presses a button found by ReadStep with a real mouse click.
func Click(p *rod.Page, b Button) error {
	el, err := p.Timeout(5 * time.Second).Element(fmt.Sprintf(`[data-pf-btn=%q]`, b.ID))
	if err != nil {
		return fmt.Errorf("button %q is gone", b.Text)
	}
	el = el.Timeout(15 * time.Second)
	_ = el.ScrollIntoView()
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		// Overlays sometimes swallow the mouse event; fall back to a DOM click.
		return evalOn(el, `function () { this.click(); }`, nil)
	}
	return nil
}

// ClickText clicks the first visible button or link whose text matches re
// (a JS regular expression, case-insensitive). It reports whether one was found.
func ClickText(p *rod.Page, re string) (string, bool) {
	var text string
	_ = Eval(p, `(re) => {
		const rx = new RegExp(re, 'i');
		const shown = el => { const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
		const el = [...document.querySelectorAll('button, a, [role="button"]')]
			.find(el => shown(el) && !el.disabled && rx.test((el.innerText || el.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim()));
		if (!el) return '';
		el.dataset.pfBtn = el.dataset.pfBtn || ('b' + (window.__pfSeq = (window.__pfSeq || 0) + 1));
		return el.dataset.pfBtn + '|' + (el.innerText || el.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim();
	}`, &text, re)
	id, label, ok := strings.Cut(text, "|")
	if !ok {
		return "", false
	}
	return label, Click(p, Button{ID: id, Text: label}) == nil
}

// HasText reports a visible button or link whose text matches re, without
// clicking it.
func HasText(p *rod.Page, re string) bool {
	var ok bool
	_ = Eval(p, `(re) => {
		const rx = new RegExp(re, 'i');
		return [...document.querySelectorAll('button, a, [role="button"]')].some(el => {
			const r = el.getBoundingClientRect();
			return r.width > 0 && r.height > 0 && rx.test((el.innerText || el.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim());
		});
	}`, &ok, re)
	return ok
}

// PageText is the visible text of the page, shortened.
func PageText(p *rod.Page, n int) string {
	var s string
	_ = Eval(p, `(n) => (document.body ? document.body.innerText : '').replace(/\n{3,}/g, '\n\n').slice(0, n)`, &s, n)
	return s
}

func evalOn(el *rod.Element, fn string, out any, args ...any) error {
	res, err := el.Eval(fn, args...)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return res.Value.Unmarshal(out)
}

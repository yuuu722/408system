"""Test routing logic without installing optional Paddle models or loading them."""
import ast
from pathlib import Path
import re
import unittest

source = ast.parse(Path(__file__).with_name('server.py').read_text(encoding='utf-8'))
function = next(n for n in source.body if isinstance(n, ast.FunctionDef) and n.name == 'uncertain_text_layer')
namespace = {'re': re}
exec(compile(ast.Module(body=[function], type_ignores=[]), '<routing>', 'exec'), namespace)
needs_layout = namespace['uncertain_text_layer']

class TextLayerTests(unittest.TestCase):
    def test_corrupt_symbols_trigger_math_layout_even_in_long_page(self):
        self.assertTrue(needs_layout('normal paragraph '*100 + 'N- !', []))
        self.assertFalse(needs_layout('TCP uses sequence numbers N-1.', []))

    def test_superscript_geometry_triggers_formula_engine(self):
        chars = [{'size':12,'x1':50,'top':20}, {'size':8,'x0':50,'top':16}]
        self.assertTrue(needs_layout('212 bytes', chars))
        self.assertFalse(needs_layout('212 bytes', [dict(c,size=12,top=20) for c in chars]))

if __name__=='__main__':unittest.main()

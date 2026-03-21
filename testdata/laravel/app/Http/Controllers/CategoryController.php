<?php

namespace App\Http\Controllers;

use App\Models\Category;
use Illuminate\Http\Request;

class CategoryController extends Controller
{
    public function index(Request $request)
    {
        $categories = Category::query()->with('products')->orderBy('name')->get();

        Category::query()->where('slug', $request->input('slug'))->first();

        return $categories;
    }

    public function show(int $id)
    {
        return Category::query()->find($id);
    }
}

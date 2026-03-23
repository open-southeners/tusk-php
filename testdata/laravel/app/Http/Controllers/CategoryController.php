<?php

namespace App\Http\Controllers;

use App\Models\Category;
use App\Models\Product;
use Illuminate\Http\Request;
use Illuminate\Support\Arr;
use Illuminate\Support\Collection;

class CategoryController extends Controller
{
    public function __construct(
        private Request $request
    ) {}

    public function index()
    {
        // Container resolution
        $configRepo = app('config');

        // Config dot-notation
        $appName = config('app.name');
        $dbConnections = config('database.connections');

        // Eloquent static magic methods
        $category = Category::first();
        $found = Category::find(1);
        $allCategories = Category::all();

        // Eloquent query chain with generics
        $queried = Category::query()
            ->where('name', 'test')
            ->orderBy('name')
            ->get();

        // Chain to first() — should resolve to ?Category
        $firstFromQuery = Category::query()->get()->first();

        // Chain to all() — should resolve to array<int, Category>
        $asArray = Category::query()->get()->all();

        // Array index on generic array
        $firstFromArray = $asArray[0];

        // Relation access after first()
        $products = $category->products;
        $productName = $category->name;

        // Array literal shape (associative)
        $employee = ['id' => 1, 'name' => 'Ruben R', 'role' => 'admin'];

        // Array of shapes (indexed)
        $employeesArr = [
            ['id' => 1, 'name' => 'Ruben R', 'role' => 'admin'],
            ['id' => 2, 'name' => 'Jorge M', 'role' => 'admin'],
        ];

        // Collection from constructor with array arg
        $col = new Collection($employeesArr);

        // Collection first() on constructed collection
        $firstEmployee = $col->first();

        // Arr::first() infers TValue from argument
        $firstViaArr = Arr::first($employeesArr);

        // Variable propagation: query -> get -> variable -> first
        $cats = Category::query()->get();
        $firstCat = $cats->first();

        // Product chain (different model)
        $productCollection = Product::query()->get();
        $firstProduct = $productCollection->first();

        return $queried;
    }

    public function show(int $id)
    {
        return Category::query()->find($id);
    }
}
